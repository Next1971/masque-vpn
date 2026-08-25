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
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"

	"masque-client/internal/engine"
	"masque-client/internal/ipc"
	"masque-client/internal/store"
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
		if err := run(debug.New(svcName)); err != nil {
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
	_ = os.MkdirAll(dir, 0700)
	f, err := os.OpenFile(filepath.Join(dir, "masque-svc.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type masqueService struct{}

func (m *masqueService) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}
	errCh := make(chan error, 1)
	go func() { errCh <- run(nil) }()
	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

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
			}
		}
	}
}

type runner interface {
	// debug.Log implements this subset
}

func run(_ debug.Log) error {
	if el, err := eventlog.Open(svcName); err == nil {
		_ = el.Info(1, "MASQUE VPN service starting")
		el.Close()
	}

	hub := ipc.NewHub()
	eng := engine.New(hub)

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
		AssignedIP:  snap.AssignedIP,
		RTTMs:       snap.RTTMs,
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
	default:
		resp.OK = false
		resp.Error = "unknown command " + req.Cmd
		return resp
	}
}
