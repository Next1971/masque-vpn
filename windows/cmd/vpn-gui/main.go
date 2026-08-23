//go:build windows

package main

import (
	"io"
	"os"
	"path/filepath"
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

func main() {
	a := app.NewWithID("com.masque.vpn")
	w := a.NewWindow("MASQUE VPN")
	w.Resize(fyne.NewSize(480, 360))

	status := widget.NewLabel("Checking service…")
	status.Wrapping = fyne.TextWrapWord
	detail := widget.NewLabel("")
	detail.Wrapping = fyne.TextWrapWord

	var auto *widget.Check
	auto = widget.NewCheck("Connect automatically when the service starts", func(v bool) {
		_, _ = ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdSetAutoconnect, Autoconnect: &v})
		refresh(status, detail, auto)
	})

	connectBtn := widget.NewButton("Connect", func() {
		_, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdConnect})
		if err != nil {
			dialog.ShowError(err, w)
		}
		refresh(status, detail, auto)
	})
	disconnectBtn := widget.NewButton("Disconnect", func() {
		_, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdDisconnect})
		if err != nil {
			dialog.ShowError(err, w)
		}
		refresh(status, detail, auto)
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
			refresh(status, detail, auto)
		}, w)
		d.Show()
	})

	w.SetContent(container.NewPadded(container.NewVBox(
		widget.NewRichTextFromMarkdown("## MASQUE VPN"),
		status,
		detail,
		layout.NewSpacer(),
		importBtn,
		container.NewGridWithColumns(2, connectBtn, disconnectBtn),
		auto,
		widget.NewLabel("The tunnel runs in a Windows service. Closing this window does not disconnect."),
	)))

	if desk, ok := a.(desktop.App); ok {
		desk.SetSystemTrayMenu(fyne.NewMenu("MASQUE",
			fyne.NewMenuItem("Show", func() { w.Show() }),
			fyne.NewMenuItem("Connect", func() { _, _ = ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdConnect}) }),
			fyne.NewMenuItem("Disconnect", func() { _, _ = ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdDisconnect}) }),
		))
		w.SetCloseIntercept(func() { w.Hide() })
	}

	go func() {
		for {
			refresh(status, detail, auto)
			time.Sleep(2 * time.Second)
		}
	}()

	w.ShowAndRun()
}

func refresh(status, detail *widget.Label, auto *widget.Check) {
	resp, err := ipc.RoundTrip(ipc.Request{Cmd: ipc.CmdStatus})
	if err != nil {
		status.SetText("Service unavailable")
		detail.SetText(err.Error() + "\nInstall the MSI and ensure the MASQUE VPN service is running.")
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
	detail.SetText(resp.Detail)
	auto.SetChecked(resp.Autoconnect)
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
