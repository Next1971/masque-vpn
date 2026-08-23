package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.zx2c4.com/wireguard/tun"
	clientcore "masque-client/internal/clientcore"
)

func main() {
	// 1. Restore command-line flags.
	profilePath := flag.String("profile", "", "path to client profile TOML (required)")
	testMode := flag.Bool("test", true, "test mode: route only -test-dst via TUN")
	fullRoute := flag.Bool("full-route", false, "full mode: route all traffic via TUN")
	testDst := flag.String("test-dst", "1.1.1.1", "test-mode: destination to route through tunnel")
	pingCount := flag.Int("ping", 3, "test-mode: number of ICMP echo requests to send")
	timeout := flag.Duration("timeout", 25*time.Second, "overall timeout")
	insecure := flag.Bool("insecure", false, "disable server certificate verification (INSECURE; troubleshooting only)")
	svcStatus := flag.Bool("svc-status", false, "query the MASQUE Windows service")
	svcConnect := flag.Bool("svc-connect", false, "ask the service to connect")
	svcDisconnect := flag.Bool("svc-disconnect", false, "ask the service to disconnect")
	svcImport := flag.String("svc-import", "", "import a profile file via the Windows service")
	flag.Parse()

	if *svcStatus || *svcConnect || *svcDisconnect || *svcImport != "" {
		if err := runServiceCLI(*svcStatus, *svcConnect, *svcDisconnect, *svcImport); err != nil {
			log.Fatal(err)
		}
		return
	}

	// 2. If -profile is supplied, run in the console (standalone; needs admin).
	if *profilePath != "" {
		prof, err := clientcore.LoadProfile(*profilePath)
		if err != nil {
			log.Fatalf("FAIL: load profile: %v", err)
		}
		// The -insecure flag overrides the profile setting when provided.
		if *insecure {
			prof.Insecure = true
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()

		err = run(ctx, prof, *testMode, *fullRoute, *testDst, *pingCount)
		if err != nil {
			log.Fatalf("FAIL: %v", err)
		}
		return
	}

	log.Println("Usage: vpn-client -profile profile.toml -full-route")
	log.Println("Or talk to the Windows service: vpn-client -svc-status | -svc-connect | -svc-import file")
}

func run(ctx context.Context, prof *clientcore.Profile, testMode, fullRoute bool, testDst string, pingCount int) error {
	// 1. Create the TUN interface (a Linux platform detail).
	dev, err := tun.CreateTUN(prof.TUNName, prof.MTU)
	if err != nil {
		return fmt.Errorf("create TUN %q: %w", prof.TUNName, err)
	}
	name, _ := dev.Name()
	log.Printf("TUN %s created (mtu %d)", name, prof.MTU)

	defer func() {
		dev.Close()
		log.Printf("TUN %s closed", name)
	}()

	// 2. Connect with the core (QUIC + mTLS + CONNECT-IP).

	sess, err := clientcore.Connect(ctx, prof, dev)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// 3. Bring up the interface with the assigned address.
	if len(sess.AssignedPrefixes) == 0 {
		sess.Close()
		return fmt.Errorf("no address assigned")
	}
	clientAddr := sess.AssignedPrefixes[0]
	if err := ifUp(name, clientAddr); err != nil {
		sess.Close()
		return fmt.Errorf("bring up %s: %w", name, err)
	}
	log.Printf("interface %s up with %s", name, clientAddr)

	// 4. Routing.
	var cleanup func()
	if fullRoute {
		cleanup, err = setupFullRoute(name, prof.Server, clientAddr.Addr(), prof.DNS)
		if err != nil {
			sess.Close()
			return fmt.Errorf("setup full route: %w", err)
		}
		log.Printf("full-route mode: all traffic via %s", name)
	} else {
		dst, perr := netip.ParseAddr(testDst)
		if perr != nil {
			sess.Close()
			return fmt.Errorf("parse test-dst %q: %w", testDst, perr)
		}
		cleanup, err = setupTestRoute(name, dst, clientAddr.Addr())
		if err != nil {
			sess.Close()
			return fmt.Errorf("setup test route: %w", err)
		}
		log.Printf("test mode: routing %s/32 via %s (default route untouched)", dst, name)
	}
	defer cleanup()

	// 5. Start forwarding in the background.
	runErr := make(chan error, 1)
	go func() { runErr <- sess.Run(ctx) }()

	// 6a. Test mode: send ICMP echo and wait for a reply through the host TUN.
	if testMode && !fullRoute {
		if err := runPingTest(ctx, testDst, name, pingCount); err != nil {
			log.Printf("ping test note: %v", err)
		}
		sess.Close()
		<-runErr
		return nil
	}

	// 6b. Full mode: run until a signal arrives.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		log.Printf("signal received, shutting down")
	case err := <-runErr:
		sess.Close()
		return err
	case <-ctx.Done():
	}
	sess.Close()
	<-runErr
	return nil
}
