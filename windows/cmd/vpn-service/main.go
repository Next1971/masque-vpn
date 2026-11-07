//go:build windows

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"

	"masque-client/internal/engine"
	"masque-client/internal/ipc"
	"masque-client/internal/store"
	"masque-client/internal/version"
)

const svcName = "MasqueVpn"

func main() {
	console := flag.Bool("console", false, "run in the foreground (debug; still needs admin for Wintun)")
	flag.Parse()

	setupLog()

	isSvc, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("IsWindowsService: %v", err)
	}
	if *console || !isSvc {
		if err := run(nil); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := svc.Run(svcName, &masqueService{}); err != nil {
		log.Fatal(err)
	}
}

func setupLog() {
	dir := store.Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		eventNote(fmt.Sprintf("MASQUE log directory: %v", err))
		return
	}
	path := filepath.Join(dir, "masque-svc.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		eventNote(fmt.Sprintf("MASQUE log file: %v", err))
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func eventNote(msg string) {
	if el, err := eventlog.Open(svcName); err == nil {
		_ = el.Info(1, msg)
		el.Close()
	}
}

type masqueService struct{}

// Power broadcast values from WinUser.h. Resume is delivered twice: once
// automatically, then again if an interactive user is logged on.
const (
	pbtApmResumeCritical  = 6
	pbtApmResumeSuspend   = 7
	pbtApmResumeAutomatic = 18
)

func powerResume(eventType uint32) bool {
	switch eventType {
	case pbtApmResumeAutomatic, pbtApmResumeSuspend, pbtApmResumeCritical:
		return true
	default:
		return false
	}
}

func (m *masqueService) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}
	resume := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() { errCh <- run(resume) }()
	const accepts = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPowerEvent
	s <- svc.Status{State: svc.Running, Accepts: accepts}

	for {
		select {
		case err := <-errCh:
			if err != nil {
				log.Printf("service loop: %v", err)
				return true, 1
			}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				os.Exit(0)
			case svc.PowerEvent:
				if powerResume(c.EventType) {
					msg := fmt.Sprintf("power resume (%d)", c.EventType)
					log.Printf("%s", msg)
					select {
					case resume <- msg:
					default:
					}
				}
			}
		}
	}
}

func run(resume <-chan string) error {
	if el, err := eventlog.Open(svcName); err == nil {
		_ = el.Info(1, "MASQUE VPN "+version.Label()+" service starting")
		el.Close()
	}
	log.Printf("MASQUE VPN %s starting", version.Label())

	hub := ipc.NewHub()
	eng := engine.New(hub)
	if resume != nil {
		go func() {
			for why := range resume {
				eng.NotifyResume(why)
			}
		}()
	}

	ln, err := ipc.Listen()
	if err != nil {
		return fmt.Errorf("listen pipe: %w", err)
	}
	defer ln.Close()
	log.Printf("IPC listening on %s", ipc.PipeName)

	go acceptLoop(ln, hub, eng)
	time.Sleep(200 * time.Millisecond)
	eng.MaybeAutoconnect()

	select {}
}

func acceptLoop(ln net.Listener, hub *ipc.Hub, eng *engine.Engine) {
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("pipe accept: %v", err)
			return
		}
		hub.Add(c)
		go handleConn(c, hub, eng)
	}
}

func handleConn(c net.Conn, hub *ipc.Hub, eng *engine.Engine) {
	defer func() {
		hub.Remove(c)
		c.Close()
	}()
	dec := json.NewDecoder(c)
	for {
		var req ipc.Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		resp := handle(eng, req)
		if err := ipc.WriteJSON(c, resp); err != nil {
			return
		}
	}
}

func handle(eng *engine.Engine, req ipc.Request) ipc.Response {
	snap := eng.Snapshot()
	resp := ipc.Response{
		ID:          req.ID,
		OK:          true,
		State:       snap.State,
		Detail:      snap.Detail,
		Configured:  snap.Configured,
		Autoconnect: snap.Autoconnect,
		KillSwitch:  snap.KillSwitch,
		AssignedIP:  snap.AssignedIP,
		RTTMs:       snap.RTTMs,
		Transport:   snap.Transport,
		MTU:         snap.MTU,
		DayBytes:    snap.DayBytes,
		MonthBytes:  snap.MonthBytes,
	}
	switch req.Cmd {
	case ipc.CmdStatus:
		return resp
	case ipc.CmdImport:
		if err := eng.Import(req.Text, req.Filename, req.CA, req.Cert, req.Key); err != nil {
			resp.OK = false
			resp.Error = err.Error()
			resp.State = ipc.StateError
			resp.Detail = err.Error()
			return resp
		}
		snap = eng.Snapshot()
		resp.Configured = snap.Configured
		resp.Detail = snap.Detail
		return resp
	case ipc.CmdConnect:
		if err := eng.Connect(); err != nil {
			resp.OK = false
			resp.Error = err.Error()
			return resp
		}
		snap = eng.Snapshot()
		resp.State = snap.State
		resp.Detail = snap.Detail
		resp.AssignedIP = snap.AssignedIP
		resp.RTTMs = snap.RTTMs
		return resp
	case ipc.CmdDisconnect:
		eng.Disconnect()
		resp.State = ipc.StateDisconnected
		resp.Detail = "disconnect requested"
		return resp
	case ipc.CmdSetAutoconnect:
		if req.Autoconnect == nil {
			resp.OK = false
			resp.Error = "missing autoconnect"
			return resp
		}
		if err := eng.SetAutoconnect(*req.Autoconnect); err != nil {
			resp.OK = false
			resp.Error = err.Error()
			return resp
		}
		resp.Autoconnect = *req.Autoconnect
		return resp
	case ipc.CmdSetKillSwitch:
		if req.KillSwitch == nil {
			resp.OK = false
			resp.Error = "missing kill_switch"
			return resp
		}
		if err := eng.SetKillSwitch(*req.KillSwitch); err != nil {
			resp.OK = false
			resp.Error = err.Error()
			return resp
		}
		resp.KillSwitch = *req.KillSwitch
		return resp
	case ipc.CmdSetTransport:
		if err := eng.SetTransport(req.Transport); err != nil {
			resp.OK = false
			resp.Error = err.Error()
			return resp
		}
		resp.Transport = store.LoadSettings().EffectiveTransport()
		return resp
	case ipc.CmdSetMTU:
		if req.MTU == 0 {
			resp.OK = false
			resp.Error = "missing mtu"
			return resp
		}
		if err := eng.SetMTU(req.MTU); err != nil {
			resp.OK = false
			resp.Error = err.Error()
			return resp
		}
		resp.MTU = store.LoadSettings().EffectiveMTU()
		return resp
	default:
		resp.OK = false
		resp.Error = "unknown command " + req.Cmd
		return resp
	}
}
