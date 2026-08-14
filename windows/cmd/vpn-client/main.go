package main

import (
    "context"
   
    "flag"
    "fmt"
    "io"
    "log"
    "net/http"
    "net/netip"
    "os"
    "os/signal"
    "syscall"
    "time"

     clientcore "masque-client/internal/clientcore"
    "golang.zx2c4.com/wireguard/tun"
)



var (
    globalCancel context.CancelFunc
    globalProf   *clientcore.Profile
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
    flag.Parse()

    // 2. If -profile is supplied, run in the console.
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

    // 3. If no flags are supplied, start the web interface.
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        data, err := os.ReadFile("index.html")
        if err != nil {
            http.Error(w, "Unable to read index.html", 500)
            return
        }
        w.Write(data)
    })

    http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "POST" {
            http.Error(w, "Method not allowed", 405)
            return
        }
        file, _, err := r.FormFile("file")
        if err != nil {
            http.Error(w, "File read error", 400)
            return
        }
        defer file.Close()

        tempPath := "uploaded_profile.toml"
        out, err := os.Create(tempPath)
        if err != nil {
            http.Error(w, "Unable to save file", 500)
            return
        }
        defer out.Close()
        io.Copy(out, file)

        prof, err := clientcore.LoadProfile(tempPath)
        if err != nil {
            http.Error(w, "TOML parsing error: "+err.Error(), 400)
            return
        }
        globalProf = prof
        w.WriteHeader(200)
    })

    http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
        if globalProf == nil {
            w.Write([]byte(`{"status":"error", "error":"Upload a configuration first"}`))
            return
        }
        if globalCancel != nil {
            w.Write([]byte(`{"status":"error", "error":"Already connected"}`))
            return
        }

        // "Disable certificate verification" checkbox from the web UI.
        // insecure=1 turns off server certificate validation for this session.
        globalProf.Insecure = r.URL.Query().Get("insecure") == "1"

        ctx, cancel := context.WithCancel(context.Background())
        globalCancel = cancel

        go func() {
            err := run(ctx, globalProf, false, true, "1.1.1.1", 3)
            if err != nil {
                log.Printf("VPN Error: %v", err)
            }
            globalCancel = nil
        }()

        w.Write([]byte(`{"status":"ok", "ip":"Connected"}`))
    })

    http.HandleFunc("/disconnect", func(w http.ResponseWriter, r *http.Request) {
        if globalCancel != nil {
            globalCancel()
            globalCancel = nil
        }
        w.Write([]byte(`{"status":"ok"}`))
    })

    log.Println("Web interface started. Open http://localhost:8080 in a browser")
    log.Fatal(http.ListenAndServe(":8080", nil))
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
