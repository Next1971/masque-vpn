package vpssetup

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed assets/issue-client.sh
var issueClientScript []byte

const (
	// FirstAppIndex is the first CN the Windows app issues (masque-client-9).
	// Numbers 1–8 are reserved for existing test bundles and are never reused.
	FirstAppIndex = 9
	// PoolSlots is the IPv4 /24 client pool size (253 assignable /32s).
	PoolSlots = 253
	// LastAppIndex is FirstAppIndex + PoolSlots - 1.
	LastAppIndex = 261

	remoteIssue = "/tmp/masque-issue-client.sh"
	remoteGen   = "/opt/masque/gen-config.sh"
)

// IssueStatus is the app-side counter (not live VPN sessions).
type IssueStatus struct {
	NextIndex int
	AppIssued int
	PoolSlots int
	Ready     bool
	Detail    string
}

func (s IssueStatus) Label() string {
	if !s.Ready {
		return s.Detail
	}
	return fmt.Sprintf("Test #1–8 reserved (do not reissue). Next #%d. App-issued %d/%d",
		s.NextIndex, s.AppIssued, s.PoolSlots)
}

// ReadIssueStatus loads /opt/masque/state/next_index (creates 9 if missing but CA exists).
func ReadIssueStatus(c *Client) (IssueStatus, error) {
	s := IssueStatus{PoolSlots: PoolSlots, NextIndex: FirstAppIndex}
	out, err := c.Run(20*time.Second, `set +e
if [ ! -d /opt/masque ]; then echo 'NONE'; exit 0; fi
if [ ! -f /opt/masque/ca/ca.key ]; then echo 'NOCA'; exit 0; fi
install -d -m 0755 /opt/masque/state
if [ ! -f /opt/masque/state/next_index ]; then echo 9 > /opt/masque/state/next_index; fi
echo READY
cat /opt/masque/state/next_index
`, nil)
	if err != nil {
		return s, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "NONE" {
		s.Detail = "Issue status: install the server first"
		return s, nil
	}
	if lines[0] == "NOCA" {
		s.Detail = "Issue status: CA key missing on VPS (/opt/masque/ca)"
		return s, nil
	}
	nstr := strings.TrimSpace(lines[len(lines)-1])
	n, err := strconv.Atoi(nstr)
	if err != nil {
		return s, fmt.Errorf("parse next_index %q", nstr)
	}
	s.Ready = true
	s.NextIndex = n
	if n >= FirstAppIndex {
		s.AppIssued = n - FirstAppIndex
	}
	s.Detail = s.Label()
	return s, nil
}

// IssueNextBundle creates masque-client-N (N>=9), increments the counter, writes profile.masque to destDir.
func IssueNextBundle(c *Client, destDir string, log Logf) (int, error) {
	st, err := ReadIssueStatus(c)
	if err != nil {
		return 0, err
	}
	if !st.Ready {
		return 0, fmt.Errorf("%s", st.Detail)
	}
	if st.NextIndex > LastAppIndex {
		return 0, fmt.Errorf("app bundle slots full (%d issued from #%d)", PoolSlots, FirstAppIndex)
	}
	if log != nil {
		log("Uploading issuer scripts…")
	}
	if err := c.Upload(remoteGen, genConfigScript, 0o755); err != nil {
		return 0, err
	}
	if err := c.Upload(remoteIssue, issueClientScript, 0o755); err != nil {
		return 0, err
	}
	if log != nil {
		log("Issuing masque-client-%d (server cert unchanged)…", st.NextIndex)
	}
	out, err := c.Run(2*time.Minute, "bash "+shellSingleQuote(remoteIssue), nil)
	if err != nil {
		return 0, err
	}
	if log != nil && out != "" {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				log("%s", line)
			}
		}
	}
	idx := st.NextIndex
	if strings.Contains(out, "ISSUED ") {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ISSUED ") {
				if n, e := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "ISSUED "))); e == nil {
					idx = n
				}
			}
		}
	}
	raw, err := c.RunBytes(30*time.Second, "cat "+shellSingleQuote(fmt.Sprintf("/opt/masque/clients/%d/profile.masque", idx)), nil)
	if err != nil {
		return idx, fmt.Errorf("download profile: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return idx, err
	}
	name := filepath.Join(destDir, fmt.Sprintf("masque-client-%d.profile.masque", idx))
	if err := os.WriteFile(name, raw, 0o600); err != nil {
		return idx, err
	}
	if log != nil {
		log("Wrote %s", name)
	}
	return idx, nil
}

// PullBootstrapProfile copies the install-time profile.masque (CN masque-client, not #9+).
func PullBootstrapProfile(c *Client, destDir string, log Logf) error {
	if log != nil {
		log("Downloading bootstrap profile.masque…")
	}
	raw, err := c.RunBytes(30*time.Second, "cat /opt/masque/generated/android/profile.masque", nil)
	if err != nil {
		return fmt.Errorf("download bootstrap profile: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(destDir, "profile.masque")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	if log != nil {
		log("Wrote %s (bootstrap CN masque-client; CA key stays on the VPS)", path)
	}
	return nil
}
