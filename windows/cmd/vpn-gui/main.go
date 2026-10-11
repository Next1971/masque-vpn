//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"masque-client/internal/ipc"
)

//go:embed icon.png
var iconPNG []byte

func main() {
	a := app.NewWithID("com.masque.vpn")
	icon := fyne.NewStaticResource("icon.png", iconPNG)
	a.SetIcon(icon)
	w := a.NewWindow("MASQUE VPN")
	w.SetIcon(icon)
	w.Resize(fyne.NewSize(480, 440))

	status := widget.NewLabel("Checking service…")
	status.Wrapping = fyne.TextWrapWord
	detail := widget.NewLabel("")
	detail.Wrapping = fyne.TextWrapWord
	ping := widget.NewLabel("Ping: —")
	traffic := widget.NewLabel("Today: 0.0 MB\nMonth: 0.0 MB")

	var auto *widget.Check
	auto = widget.NewCheck("Connect automatically when the service starts", func(v bool) {
		_, _ = ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdSetAutoconnect, Autoconnect: &v})
		refresh(status, detail, ping, traffic, auto)
	})

	connectBtn := widget.NewButton("Connect", func() {
		_, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdConnect})
		if err != nil {
			dialog.ShowError(err, w)
		}
		refresh(status, detail, ping, traffic, auto)
	})
	disconnectBtn := widget.NewButton("Disconnect", func() {
		_, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdDisconnect})
		if err != nil {
			dialog.ShowError(err, w)
		}
		refresh(status, detail, ping, traffic, auto)
	})
	importBtn := widget.NewButton("Import profile", func() {
		d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if rc == nil {
				return
			}
			defer rc.Close()
			name := rc.URI().Name()
			data, rerr := io.ReadAll(rc)
			if rerr != nil {
				dialog.ShowError(rerr, w)
				return
			}
			ca, cert, key := companionPEMs(rc.URI().Path(), string(data))
			_, ierr := ipc.RoundTrip(ipc.Request{
				Cmd:      ipc.CmdImport,
				Text:     string(data),
				Filename: name,
				CA:       ca,
				Cert:     cert,
				Key:      key,
			})
			if ierr != nil {
				dialog.ShowError(ierr, w)
				return
			}
			refresh(status, detail, ping, traffic, auto)
		}, w)
		d.Show()
	})
	settingsBtn := widget.NewButton("Settings", func() {
		showSettings(w)
	})

	w.SetContent(container.NewPadded(container.NewVBox(
		widget.NewRichTextFromMarkdown("## MASQUE VPN"),
		status,
		ping,
		traffic,
		detail,
		layout.NewSpacer(),
		importBtn,
		container.NewGridWithColumns(2, connectBtn, disconnectBtn),
		settingsBtn,
		auto,
		widget.NewLabel("The tunnel runs in a Windows service. Closing this window does not disconnect."),
	)))

	if desk, ok := a.(desktop.App); ok {
		desk.SetSystemTrayIcon(icon)
		desk.SetSystemTrayMenu(fyne.NewMenu("MASQUE",
			fyne.NewMenuItem("Show", func() { w.Show() }),
			fyne.NewMenuItem("Connect", func() { _, _ = ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdConnect}) }),
			fyne.NewMenuItem("Disconnect", func() { _, _ = ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdDisconnect}) }),
		))
		w.SetCloseIntercept(func() { w.Hide() })
	}

	go func() {
		for {
			refresh(status, detail, ping, traffic, auto)
			time.Sleep(2 * time.Second)
		}
	}()

	w.ShowAndRun()
}

func refresh(status, detail, ping, traffic *widget.Label, auto *widget.Check) {
	resp, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdStatus})
	if err != nil {
		status.SetText("Service unavailable")
		detail.SetText(err.Error() + "\nInstall the MSI and ensure the MASQUE VPN service is running.")
		ping.SetText("Ping: —")
		traffic.SetText("Today: —\nMonth: —")
		return
	}
	line := "State: " + resp.State
	if resp.AssignedIP != "" {
		line += "  (" + resp.AssignedIP + ")"
	}
	if !resp.Configured {
		line += " — import a profile first"
	}
	status.SetText(line)
	if resp.State == ipc.StateConnected && resp.RTTMs > 0 {
		via := "QUIC to server"
		if strings.Contains(resp.Detail, "via tcp") {
			via = "TCP to server"
		}
		ping.SetText(fmt.Sprintf("Ping: %d ms  (%s)", resp.RTTMs, via))
	} else {
		ping.SetText("Ping: —")
	}
	traffic.SetText(fmt.Sprintf("Today: %s\nMonth: %s", formatMB(resp.DayBytes), formatMB(resp.MonthBytes)))
	detail.SetText(resp.Detail)
	auto.SetChecked(resp.Autoconnect)
}

func showSettings(w fyne.Window) {
	resp, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdStatus})
	if err != nil {
		dialog.ShowError(err, w)
		return
	}
	radio := widget.NewRadioGroup([]string{
		"Automatic (UDP, then TCP)",
		"UDP only",
		"TCP only",
	}, nil)
	switch resp.Transport {
	case "udp":
		radio.SetSelected("UDP only")
	case "tcp":
		radio.SetSelected("TCP only")
	default:
		radio.SetSelected("Automatic (UDP, then TCP)")
	}
	ks := widget.NewCheck("Kill switch (block internet if the tunnel drops)", nil)
	ks.SetChecked(resp.KillSwitch)
	mtu := widget.NewEntry()
	if resp.MTU > 0 {
		mtu.SetText(strconv.Itoa(resp.MTU))
	} else {
		mtu.SetText("1340")
	}
	note := widget.NewLabel("Applies on the next connect. MTU 1280–1500, default 1340.")
	note.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm("Settings", "Save", "Cancel", container.NewVBox(
		widget.NewLabel("Transport"),
		radio,
		ks,
		widget.NewLabel("MTU"),
		mtu,
		note,
	), func(ok bool) {
		if !ok {
			return
		}
		mode := "auto"
		switch radio.Selected {
		case "UDP only":
			mode = "udp"
		case "TCP only":
			mode = "tcp"
		}
		if _, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdSetTransport, Transport: mode}); err != nil {
			dialog.ShowError(err, w)
			return
		}
		on := ks.Checked
		if _, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdSetKillSwitch, KillSwitch: &on}); err != nil {
			dialog.ShowError(err, w)
			return
		}
		n, convErr := strconv.Atoi(strings.TrimSpace(mtu.Text))
		if convErr != nil || n == 0 {
			n = 1340
		}
		if _, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdSetMTU, MTU: n}); err != nil {
			dialog.ShowError(err, w)
		}
	}, w)
	d.Resize(fyne.NewSize(420, 380))
	d.Show()
}

func formatMB(n int64) string {
	if n < 0 {
		n = 0
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1_000_000)
}

func companionPEMs(path, text string) (ca, cert, key string) {
	if strings.Contains(text, "-----BEGIN") {
		return "", "", ""
	}
	dir := filepath.Dir(path)
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			return ""
		}
		return string(b)
	}
	return read("certs/ca.crt"), read("certs/client.crt"), read("certs/client.key")
}
