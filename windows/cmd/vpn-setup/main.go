//go:build windows

package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"masque-client/internal/vpssetup"
)

//go:embed icon.png
var iconPNG []byte

func main() {
	a := app.NewWithID("com.masque.setup")
	icon := fyne.NewStaticResource("icon.png", iconPNG)
	a.SetIcon(icon)
	w := a.NewWindow("MASQUE server setup (experimental)")
	w.SetIcon(icon)
	w.Resize(fyne.NewSize(640, 720))

	sshHost := widget.NewEntry()
	sshHost.SetPlaceHolder("VPS IP or hostname")
	sshPort := widget.NewEntry()
	sshPort.SetText("22")
	sshUser := widget.NewEntry()
	sshUser.SetText("root")
	password := widget.NewPasswordEntry()
	keyPath := widget.NewEntry()
	keyPath.SetPlaceHolder("optional: path to private key")
	keyPass := widget.NewPasswordEntry()
	keyPass.SetPlaceHolder("key passphrase, if any")
	publicHost := widget.NewEntry()
	publicHost.SetPlaceHolder("same as SSH host unless clients use a DNS name")
	binPath := widget.NewEntry()
	binPath.SetPlaceHolder("optional: vpn-server-linux-amd64 or arm64")

	logBox := widget.NewMultiLineEntry()
	logBox.Wrapping = fyne.TextWrapWord
	logBox.SetMinRowsVisible(12)

	portSelect := widget.NewSelect([]string{}, nil)
	portSelect.PlaceHolder = "Connect first"
	customPort := widget.NewEntry()
	customPort.SetPlaceHolder("or type a UDP port")
	probeLabel := widget.NewLabel("Reachability: —")
	probeLabel.Wrapping = fyne.TextWrapWord
	issueLabel := widget.NewLabel("Issue status: connect first")
	issueLabel.Wrapping = fyne.TextWrapWord

	var (
		cli       *vpssetup.Client
		pre       vpssetup.Preflight
		confirmed int
		busy      bool
		exeDir    string
	)
	if p, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(p)
	}

	logf := func(format string, args ...interface{}) {
		line := fmt.Sprintf(format, args...)
		logBox.SetText(strings.TrimSpace(logBox.Text + "\n" + line))
	}

	knownPath := filepath.Join(configDir(), "setup_known_hosts")

	setBusy := func(v bool) { busy = v }

	authFromForm := func() (vpssetup.Auth, error) {
		a := vpssetup.Auth{User: sshUser.Text, Password: password.Text, KeyPassword: keyPass.Text}
		if strings.TrimSpace(keyPath.Text) != "" {
			b, err := os.ReadFile(keyPath.Text)
			if err != nil {
				return a, err
			}
			a.KeyPEM = b
		}
		return a, nil
	}

	dialWithTrust := func() (*vpssetup.Client, error) {
		host := strings.TrimSpace(sshHost.Text)
		if err := vpssetup.ValidateHost(host); err != nil {
			return nil, err
		}
		sp, err := strconv.Atoi(strings.TrimSpace(sshPort.Text))
		if err != nil {
			return nil, fmt.Errorf("SSH port")
		}
		au, err := authFromForm()
		if err != nil {
			return nil, err
		}
		c, err := vpssetup.Dial(host, sp, au, knownPath)
		var unk *vpssetup.UnknownHostError
		if errors.As(err, &unk) {
			return nil, err
		}
		return c, err
	}

	connectBtn := widget.NewButton("1. Connect and check OS", func() {
		if busy {
			return
		}
		setBusy(true)
		go func() {
			defer setBusy(false)
			if cli != nil {
				_ = cli.Close()
				cli = nil
			}
			host := strings.TrimSpace(sshHost.Text)
			c, err := dialWithTrust()
			var unk *vpssetup.UnknownHostError
			if errors.As(err, &unk) {
				fyne.Do(func() {
					dialog.ShowConfirm("Unknown SSH host",
						fmt.Sprintf("Fingerprint:\n%s\n\nOnly accept this if it matches your VPS.", unk.Fingerprint),
						func(ok bool) {
							if !ok {
								logf("SSH host key not accepted")
								return
							}
							if aerr := vpssetup.AppendKnownHost(knownPath, unk.Host, unk.Key); aerr != nil {
								dialog.ShowError(aerr, w)
								return
							}
							c2, err2 := dialWithTrust()
							if err2 != nil {
								dialog.ShowError(err2, w)
								return
							}
							finishPreflight(c2, w, logf, &cli, &pre, &confirmed, portSelect, publicHost, host, issueLabel, probeLabel, confirmBtn, installBtn)
						}, w)
				})
				return
			}
			if err != nil {
				fyne.Do(func() { dialog.ShowError(err, w) })
				return
			}
			finishPreflight(c, w, logf, &cli, &pre, &confirmed, portSelect, publicHost, host, issueLabel, probeLabel, confirmBtn, installBtn)
		}()
	})

	confirmBtn := widget.NewButton("2. Confirm UDP port", func() {
		if busy {
			return
		}
		p, err := chosenPort(portSelect, customPort)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		if cli == nil {
			dialog.ShowError(fmt.Errorf("connect first"), w)
			return
		}
		if _, taken := pre.Listening[p]; taken {
			dialog.ShowError(fmt.Errorf("UDP %d is already listening on the VPS", p), w)
			return
		}
		confirmed = p
		logf("Port confirmed: UDP %d", p)
		probeLabel.SetText("Reachability: — (install first)")
	})

	installBtn := widget.NewButton("3. Install MASQUE server", func() {
		if busy {
			return
		}
		if cli == nil {
			dialog.ShowError(fmt.Errorf("connect first"), w)
			return
		}
		if pre.Existing.Present {
			dialog.ShowError(fmt.Errorf("%s", pre.Existing.Summary()), w)
			return
		}
		if confirmed == 0 {
			dialog.ShowError(fmt.Errorf("confirm a UDP port first"), w)
			return
		}
		pub := strings.TrimSpace(publicHost.Text)
		if pub == "" {
			pub = strings.TrimSpace(sshHost.Text)
		}
		if err := vpssetup.ValidateHost(pub); err != nil {
			dialog.ShowError(err, w)
			return
		}
		setBusy(true)
		go func() {
			defer setBusy(false)
			cwd, _ := os.Getwd()
			binFile, err := vpssetup.FindLinuxBinary(pre.GoArch, strings.TrimSpace(binPath.Text), []string{exeDir, cwd})
			if err != nil {
				fyne.Do(func() { dialog.ShowError(err, w) })
				return
			}
			fyne.Do(func() { logf("Using binary: %s", binFile) })
			raw, err := os.ReadFile(binFile)
			if err != nil {
				fyne.Do(func() { dialog.ShowError(err, w) })
				return
			}
			lg := func(format string, args ...interface{}) {
				msg := fmt.Sprintf(format, args...)
				fyne.Do(func() { logf("%s", msg) })
			}
			if err := vpssetup.Install(cli, pub, confirmed, raw, lg); err != nil {
				fyne.Do(func() { dialog.ShowError(err, w); probeLabel.SetText("Reachability: not installed") })
				return
			}
			fyne.Do(func() { logf("Service started. Probing QUIC on %s:%d …", pub, confirmed) })
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			perr := vpssetup.ProbeUDPListener(ctx, pub, confirmed)
			fyne.Do(func() {
				if perr != nil {
					probeLabel.SetText("Reachability: NOT OK — " + perr.Error())
					logf("NOT OK: %s", perr.Error())
				} else {
					probeLabel.SetText(fmt.Sprintf("Reachability: OK — QUIC answered on UDP %d", confirmed))
					logf("OK: listener is reachable from this PC")
				}
			})
		}()
	})

	saveBtn := widget.NewButton("Save bootstrap profile.masque…", func() {
		if busy || cli == nil {
			dialog.ShowError(fmt.Errorf("install (or connect to an installed server) first"), w)
			return
		}
		dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if u == nil {
				return
			}
			dest := u.Path()
			setBusy(true)
			go func() {
				defer setBusy(false)
				lg := func(format string, args ...interface{}) {
					msg := fmt.Sprintf(format, args...)
					fyne.Do(func() { logf("%s", msg) })
				}
				if err := vpssetup.PullBootstrapProfile(cli, dest, lg); err != nil {
					fyne.Do(func() { dialog.ShowError(err, w) })
					return
				}
			}()
		}, w)
	})

	issueBtn := widget.NewButton("Issue next bundle (#9+)…", func() {
		if busy || cli == nil {
			dialog.ShowError(fmt.Errorf("connect first"), w)
			return
		}
		dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if u == nil {
				return
			}
			dest := u.Path()
			setBusy(true)
			go func() {
				defer setBusy(false)
				lg := func(format string, args ...interface{}) {
					msg := fmt.Sprintf(format, args...)
					fyne.Do(func() { logf("%s", msg) })
				}
				idx, err := vpssetup.IssueNextBundle(cli, dest, lg)
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, w)
						return
					}
					logf("Issued #%d — import masque-client-%d.profile.masque (Windows GUI accepts the same file)", idx, idx)
					st, _ := vpssetup.ReadIssueStatus(cli)
					issueLabel.SetText(st.Label())
				})
			}()
		}, w)
	})

	pickKey := widget.NewButton("Browse key…", func() {
		dialog.ShowFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil || rc == nil {
				return
			}
			keyPath.SetText(rc.URI().Path())
			_ = rc.Close()
		}, w)
	})
	pickBin := widget.NewButton("Browse binary…", func() {
		dialog.ShowFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil || rc == nil {
				return
			}
			binPath.SetText(rc.URI().Path())
			_ = rc.Close()
		}, w)
	})

	form := container.NewVBox(
		widget.NewRichTextFromMarkdown("## MASQUE VPS installer (experimental)\n\n**Test / pre-release.** This can break a VPS or leak a root password if you use it carelessly. Do not treat it as a finished product. No certificate revocation."),
		widget.NewLabel("Ubuntu 22.04/24.04 or Debian 12, as root. If MASQUE is already installed, Connect skips install and opens key issuance."),
		widget.NewForm(
			widget.NewFormItem("SSH host", sshHost),
			widget.NewFormItem("SSH port", sshPort),
			widget.NewFormItem("User", sshUser),
			widget.NewFormItem("Password", password),
			widget.NewFormItem("Private key", container.NewBorder(nil, nil, nil, pickKey, keyPath)),
			widget.NewFormItem("Key passphrase", keyPass),
			widget.NewFormItem("Public host (TLS)", publicHost),
			widget.NewFormItem("Linux server binary", container.NewBorder(nil, nil, nil, pickBin, binPath)),
		),
		connectBtn,
		widget.NewLabel("Naive UDP candidates: 443, 2053, 8443, 41234 (not already listening on the VPS)."),
		container.NewGridWithColumns(2, portSelect, customPort),
		confirmBtn,
		installBtn,
		probeLabel,
		issueLabel,
		issueBtn,
		saveBtn,
		layout.NewSpacer(),
		widget.NewLabel("Log"),
		logBox,
	)

	w.SetContent(container.NewPadded(container.NewScroll(form)))
	w.ShowAndRun()
	if cli != nil {
		_ = cli.Close()
	}
}

func finishPreflight(c *vpssetup.Client, w fyne.Window, logf func(string, ...interface{}), cli **vpssetup.Client, pre *vpssetup.Preflight, confirmed *int, portSelect *widget.Select, publicHost *widget.Entry, sshHost string, issueLabel, probeLabel *widget.Label, confirmBtn, installBtn *widget.Button) {
	pf, err := vpssetup.CheckOSAndMachine(c, func(format string, args ...interface{}) {
		fyne.Do(func() { logf(format, args...) })
	})
	if err != nil {
		_ = c.Close()
		fyne.Do(func() { dialog.ShowError(err, w) })
		return
	}
	st, sterr := vpssetup.ReadIssueStatus(c)
	issueText := "Issue status: —"
	if sterr != nil {
		issueText = "Issue status: " + sterr.Error()
	} else {
		issueText = st.Label()
	}
	opts := make([]string, 0, len(pf.Recommended))
	for _, p := range pf.Recommended {
		opts = append(opts, strconv.Itoa(p))
	}
	fyne.Do(func() {
		*cli = c
		*pre = pf
		portSelect.Options = opts
		if len(opts) > 0 {
			portSelect.SetSelected(opts[0])
		} else {
			portSelect.PlaceHolder = "all candidates busy — type a port"
			portSelect.ClearSelected()
		}
		if strings.TrimSpace(publicHost.Text) == "" {
			publicHost.SetText(sshHost)
		}
		if pf.Existing.Present {
			confirmBtn.Disable()
			installBtn.Disable()
			portSelect.Disable()
			if pf.Existing.UDPPort > 0 {
				*confirmed = pf.Existing.UDPPort
			}
			if pf.Existing.ServerName != "" {
				publicHost.SetText(pf.Existing.ServerName)
			}
			probeLabel.SetText(pf.Existing.Summary() + " Use Issue next bundle.")
		} else {
			confirmBtn.Enable()
			installBtn.Enable()
			portSelect.Enable()
		}
		issueLabel.SetText(issueText)
	})
}

func chosenPort(sel *widget.Select, custom *widget.Entry) (int, error) {
	if s := strings.TrimSpace(custom.Text); s != "" {
		p, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("invalid custom port")
		}
		return p, vpssetup.ValidateUDPPort(p)
	}
	if sel.Selected == "" {
		return 0, fmt.Errorf("select or type a UDP port")
	}
	p, err := strconv.Atoi(sel.Selected)
	if err != nil {
		return 0, err
	}
	return p, vpssetup.ValidateUDPPort(p)
}

func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d, _ = os.UserHomeDir()
	}
	p := filepath.Join(d, "MASQUE")
	_ = os.MkdirAll(p, 0o700)
	return p
}
