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
	"fyne.io/fyne/v2/canvas"
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
	w.Resize(fyne.NewSize(680, 920))

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
	customPort.SetPlaceHolder("or type a listen port")
	altSelect := widget.NewSelect([]string{}, nil)
	altSelect.PlaceHolder = "Connect first"
	altCustom := widget.NewEntry()
	altCustom.SetPlaceHolder("or type a second port")
	probeLabel := widget.NewLabel("Reachability: —")
	probeLabel.Wrapping = fyne.TextWrapWord
	issueLabel := widget.NewLabel("Issue status: connect first")
	issueLabel.Wrapping = fyne.TextWrapWord

	var (
		cli          *vpssetup.Client
		pre          vpssetup.Preflight
		confirmed    int
		confirmedAlt int
		busy         bool
		exeDir       string
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

	var confirmBtn, installBtn *widget.Button

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
							finishPreflight(c2, w, logf, &cli, &pre, &confirmed, &confirmedAlt, portSelect, altSelect, altCustom, publicHost, host, issueLabel, probeLabel, confirmBtn, installBtn)
						}, w)
				})
				return
			}
			if err != nil {
				fyne.Do(func() { dialog.ShowError(err, w) })
				return
			}
			finishPreflight(c, w, logf, &cli, &pre, &confirmed, &confirmedAlt, portSelect, altSelect, altCustom, publicHost, host, issueLabel, probeLabel, confirmBtn, installBtn)
		}()
	})

	applyPorts := func(listen, alt int, conflicts []string) {
		confirmed = listen
		confirmedAlt = alt
		if alt > 0 {
			logf("Ports confirmed: UDP/TCP %d, second port %d", listen, alt)
		} else {
			logf("Ports confirmed: UDP/TCP %d (no second port)", listen)
		}
		for _, line := range conflicts {
			logf("Warning: %s", line)
		}
		probeLabel.SetText("Reachability: — (install first)")
	}
	confirmBtn = widget.NewButton("2. Confirm ports", func() {
		if busy {
			return
		}
		listen, err := chosenPort(portSelect, customPort)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		alt, err := chosenAlt(altSelect, altCustom)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		if err := vpssetup.ValidatePortPair(listen, alt); err != nil {
			dialog.ShowError(err, w)
			return
		}
		if cli == nil {
			dialog.ShowError(fmt.Errorf("connect first"), w)
			return
		}
		conflicts := vpssetup.DescribeConflicts(pre.Listening, pre.ListeningTCP, listen, alt)
		if len(conflicts) == 0 {
			applyPorts(listen, alt, nil)
			return
		}
		dialog.ShowConfirm("Port already in use",
			strings.Join(conflicts, "\n\n")+"\n\nContinue with these ports anyway? Choose No to go back and pick another port.",
			func(yes bool) {
				if !yes {
					logf("Ports not confirmed — choose another")
					return
				}
				applyPorts(listen, alt, conflicts)
			}, w)
	})

	installBtn = widget.NewButton("3. Install MASQUE server", func() {
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
			dialog.ShowError(fmt.Errorf("confirm ports first"), w)
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
			if err := vpssetup.Install(cli, pub, confirmed, confirmedAlt, raw, lg); err != nil {
				fyne.Do(func() { dialog.ShowError(err, w); probeLabel.SetText("Reachability: not installed") })
				return
			}
			ports := []int{confirmed}
			if confirmedAlt > 0 {
				ports = append(ports, confirmedAlt)
			}
			fyne.Do(func() { logf("Service started. Probing UDP and TCP from this PC…") })
			var lines []string
			for _, p := range ports {
				lines = append(lines, probeLine(pub, p, "UDP", vpssetup.ProbeUDPListener))
				lines = append(lines, probeLine(pub, p, "TCP", vpssetup.ProbeTCPListener))
			}
			text := "Reachability:\n" + strings.Join(lines, "\n")
			fyne.Do(func() {
				probeLabel.SetText(text)
				logf("%s", strings.ReplaceAll(text, "\n", " | "))
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
				raw, err := vpssetup.PullBootstrapProfile(cli, dest, lg)
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, w)
						return
					}
					showProfileQR(w, raw)
				})
			}()
		}, w)
	})

	revokeNum := widget.NewEntry()
	revokeNum.SetPlaceHolder("certificate number, e.g. 7")
	revokeBtn := widget.NewButton("Revoke certificate…", func() {
		if busy || cli == nil {
			dialog.ShowError(fmt.Errorf("connect first"), w)
			return
		}
		n, err := vpssetup.ParseCertNumber(revokeNum.Text)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		cn, err := vpssetup.ClientCN(n)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		dialog.ShowConfirm("Revoke "+cn+"?",
			"Appends this CN to /opt/masque/blocked_cns on the VPS and restarts masque.service. The certificate file is not deleted. After restart the server refuses that CN.",
			func(ok bool) {
				if !ok {
					return
				}
				setBusy(true)
				go func() {
					defer setBusy(false)
					lg := func(format string, args ...interface{}) {
						msg := fmt.Sprintf(format, args...)
						fyne.Do(func() { logf("%s", msg) })
					}
					if err := vpssetup.RevokeClient(cli, n, lg); err != nil {
						fyne.Do(func() { dialog.ShowError(err, w) })
						return
					}
					if err := vpssetup.RestartMasque(cli, lg); err != nil {
						fyne.Do(func() { dialog.ShowError(err, w) })
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
				idx, raw, err := vpssetup.IssueNextBundle(cli, dest, lg)
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(err, w)
						return
					}
					logf("Issued #%d — import masque-client-%d.profile.masque (Windows GUI accepts the same file)", idx, idx)
					st, _ := vpssetup.ReadIssueStatus(cli)
					issueLabel.SetText(st.Label())
					showProfileQR(w, raw)
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
		widget.NewRichTextFromMarkdown("## MASQUE VPS installer (experimental)\n\n**Test / pre-release.** This can break a VPS or leak a root password if you use it carelessly. Do not treat it as a finished product. Compromised client CNs can be revoked (denylist + service restart)."),
		widget.NewLabel("Ubuntu 22.04/24.04/26.04 or Debian 12, as root. If MASQUE is already installed, Connect skips install and opens key issuance."),
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
		widget.NewLabel("Listen port is UDP and TCP together. Candidates: 443, 2053, 8443, 41234. If UDP or TCP is already taken, Confirm asks whether to continue or pick another port."),
		container.NewGridWithColumns(2, portSelect, customPort),
		widget.NewLabel("Second port (optional). Redirected to the listen port for UDP and TCP, and written as alt_port. Default is the next candidate. Choose (none) for one port."),
		container.NewGridWithColumns(2, altSelect, altCustom),
		confirmBtn,
		installBtn,
		probeLabel,
		issueLabel,
		issueBtn,
		saveBtn,
		widget.NewForm(widget.NewFormItem("Revoke CN number", revokeNum)),
		revokeBtn,
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

func finishPreflight(c *vpssetup.Client, w fyne.Window, logf func(string, ...interface{}), cli **vpssetup.Client, pre *vpssetup.Preflight, confirmed, confirmedAlt *int, portSelect, altSelect *widget.Select, altCustom *widget.Entry, publicHost *widget.Entry, sshHost string, issueLabel, probeLabel *widget.Label, confirmBtn, installBtn *widget.Button) {
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
	opts := make([]string, 0, len(vpssetup.CandidatePorts))
	for _, p := range vpssetup.CandidatePorts {
		opts = append(opts, strconv.Itoa(p))
	}
	altOpts := append([]string{altNone}, opts...)
	fyne.Do(func() {
		*cli = c
		*pre = pf
		portSelect.Options = opts
		altSelect.Options = altOpts
		if len(opts) > 0 {
			portSelect.SetSelected(opts[0])
			altSelect.SetSelected(defaultAlt(opts[0]))
		}
		if strings.TrimSpace(publicHost.Text) == "" {
			publicHost.SetText(sshHost)
		}
		if pf.Existing.Present {
			confirmBtn.Disable()
			installBtn.Disable()
			portSelect.Disable()
			altSelect.Disable()
			altCustom.Disable()
			if pf.Existing.UDPPort > 0 {
				*confirmed = pf.Existing.UDPPort
			}
			*confirmedAlt = pf.Existing.AltPort
			if pf.Existing.ServerName != "" {
				publicHost.SetText(pf.Existing.ServerName)
			}
			probeLabel.SetText(pf.Existing.Summary() + " Use Issue next bundle.")
		} else {
			*confirmed = 0
			*confirmedAlt = 0
			confirmBtn.Enable()
			installBtn.Enable()
			portSelect.Enable()
			altSelect.Enable()
			altCustom.Enable()
		}
		issueLabel.SetText(issueText)
	})
}

const altNone = "(none)"

func defaultAlt(primary string) string {
	for _, p := range vpssetup.CandidatePorts {
		s := strconv.Itoa(p)
		if s != primary {
			return s
		}
	}
	return altNone
}

func chosenAlt(sel *widget.Select, custom *widget.Entry) (int, error) {
	if s := strings.TrimSpace(custom.Text); s != "" {
		p, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("invalid second port")
		}
		return p, nil
	}
	if sel.Selected == "" || sel.Selected == altNone {
		return 0, nil
	}
	p, err := strconv.Atoi(sel.Selected)
	if err != nil {
		return 0, fmt.Errorf("invalid second port")
	}
	return p, nil
}

func showProfileQR(w fyne.Window, profile []byte) {
	img, err := vpssetup.ProfileQR(profile)
	if err != nil {
		dialog.ShowError(err, w)
		return
	}
	pic := canvas.NewImageFromImage(img)
	pic.FillMode = canvas.ImageFillContain
	pic.SetMinSize(fyne.NewSize(440, 440))
	note := widget.NewLabel("This code is the profile, including the private key. Scan it in the MASQUE app on an Android phone or iPhone. The file is still saved. Android TV and the Windows client use the file.")
	note.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustom("Scan profile", "Close", container.NewVBox(note, pic), w)
	d.Resize(fyne.NewSize(520, 700))
	d.Show()
}

func probeLine(host string, port int, kind string, probe func(context.Context, string, int) error) string {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := probe(ctx, host, port); err != nil {
		return fmt.Sprintf("%s %d: NOT OK — %s", kind, port, err.Error())
	}
	return fmt.Sprintf("%s %d: OK", kind, port)
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
		return 0, fmt.Errorf("select or type a listen port")
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
