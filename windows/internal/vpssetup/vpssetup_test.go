package vpssetup

import (
	"os"
	"strings"
	"testing"
)

func TestParseOSReleaseUbuntu(t *testing.T) {
	d := ParseOSRelease(`
PRETTY_NAME="Ubuntu 22.04.5 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
ID=ubuntu
`)
	if d.ID != "ubuntu" || d.VersionID != "22.04" {
		t.Fatalf("%+v", d)
	}
	if err := Supported(d); err != nil {
		t.Fatal(err)
	}
}

func TestSupportedRejects(t *testing.T) {
	cases := []Distro{
		{ID: "ubuntu", VersionID: "20.04", PrettyName: "Ubuntu 20.04"},
		{ID: "alpine", VersionID: "3.19", PrettyName: "Alpine"},
		{ID: "debian", VersionID: "11", PrettyName: "Debian 11"},
	}
	for _, d := range cases {
		if err := Supported(d); err == nil {
			t.Fatalf("expected reject %+v", d)
		}
	}
	if err := Supported(Distro{ID: "debian", VersionID: "12"}); err != nil {
		t.Fatal(err)
	}
	if err := Supported(Distro{ID: "ubuntu", VersionID: "24.04"}); err != nil {
		t.Fatal(err)
	}
	if err := Supported(Distro{ID: "ubuntu", VersionID: "26.04"}); err != nil {
		t.Fatal(err)
	}
}

func TestGoArch(t *testing.T) {
	a, err := GoArch("x86_64")
	if err != nil || a != "amd64" {
		t.Fatalf("%s %v", a, err)
	}
	if _, err := GoArch("ppc64le"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseListeningUDPPorts(t *testing.T) {
	ss := "UNCONN 0 0 0.0.0.0:53 0.0.0.0:*\nUNCONN 0 0 *:443 *:*\n"
	m := ParseListeningUDPPorts(ss)
	if _, ok := m[53]; !ok {
		t.Fatal("53")
	}
	if _, ok := m[443]; !ok {
		t.Fatal("443")
	}
	rec := Recommend(m)
	for _, p := range rec {
		if p == 443 {
			t.Fatal("443 should not be recommended")
		}
	}
	if len(rec) != 3 {
		t.Fatalf("got %v", rec)
	}
}

func TestParseServerTOML(t *testing.T) {
	name, port := ParseServerTOML(`
[server]
bind        = "0.0.0.0:2053"
server_name = "vpn.example.com"
`)
	if name != "vpn.example.com" || port != 2053 {
		t.Fatalf("%s %d", name, port)
	}
	ex := parseMasqueProbe("HASCONFIG\nACTIVE\nHASCA\n---TOML---\nbind = \"0.0.0.0:443\"\nserver_name = \"10.0.0.1\"\n---ENDTOML---")
	if !ex.Present || !ex.Active || !ex.HasCA || ex.UDPPort != 443 || ex.ServerName != "10.0.0.1" {
		t.Fatalf("%+v", ex)
	}
}

func TestAppIssueRange(t *testing.T) {
	if LastAppIndex != FirstAppIndex+PoolSlots-1 {
		t.Fatalf("LastAppIndex=%d", LastAppIndex)
	}
	st := IssueStatus{Ready: true, NextIndex: 9, AppIssued: 0, PoolSlots: PoolSlots}
	if !strings.Contains(st.Label(), "#9") || !strings.Contains(st.Label(), "0/253") {
		t.Fatalf("%s", st.Label())
	}
}

func TestValidateHost(t *testing.T) {
	if err := ValidateHost("203.0.113.10"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHost("vpn.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHost("host;rm -rf /"); err == nil {
		t.Fatal("expected reject")
	}
	if err := ValidateUDPPort(22); err == nil {
		t.Fatal("ssh port")
	}
	if err := ValidatePortPair(443, 0); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePortPair(443, 443); err == nil {
		t.Fatal("same port")
	}
	if err := ValidatePortPair(443, 22); err == nil {
		t.Fatal("ssh alt")
	}
}

func TestDescribeConflicts(t *testing.T) {
	udp := map[int]struct{}{443: {}}
	tcp := map[int]struct{}{2053: {}}
	lines := DescribeConflicts(udp, tcp, 443, 2053)
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "UDP 443") || !strings.Contains(text, "will not start") {
		t.Fatalf("listen udp: %s", text)
	}
	if !strings.Contains(text, "TCP 2053") || !strings.Contains(text, "redirects") {
		t.Fatalf("alt tcp: %s", text)
	}
	if len(DescribeConflicts(nil, nil, 443, 0)) != 0 {
		t.Fatal("expected no conflicts")
	}
	free := RecommendFree(udp, tcp)
	for _, p := range free {
		if p == 443 || p == 2053 {
			t.Fatalf("busy port recommended: %v", free)
		}
	}
}

func TestExistingCABlocksReinstall(t *testing.T) {
	ex := parseMasqueProbe("NOCONFIG\nNOBIN\nNOSVC\nINACTIVE\nHASCA\nNOALT\n---TOML---\n---ENDTOML---")
	if !ex.Present || !ex.HasCA {
		t.Fatalf("%+v", ex)
	}
	ex = parseMasqueProbe("HASCONFIG\nACTIVE\nHASCA\nALTPORT 2053\n---TOML---\nbind = \"[::]:443\"\nserver_name = \"vpn.example.com\"\n---ENDTOML---")
	if ex.UDPPort != 443 || ex.AltPort != 2053 || ex.ServerName != "vpn.example.com" {
		t.Fatalf("%+v", ex)
	}
	if !strings.Contains(ex.Summary(), "alt 2053") {
		t.Fatalf("%s", ex.Summary())
	}
}

func TestInstallScriptKeepsExistingLayout(t *testing.T) {
	b, err := os.ReadFile("assets/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, s := range []string{
		"Refusing to replace certificates",
		"MASQUE_ALT_PORT",
		"--alt-port",
		"/tcp",
		"REDIRECT",
		"state/alt_port",
		"TCP_LISTEN_OK",
		"blocked_cns",
	} {
		if !strings.Contains(text, s) {
			t.Fatalf("install.sh missing %q", s)
		}
	}
	if strings.Contains(text, "rm -rf /opt/masque") || strings.Contains(text, "rm ") {
		t.Fatal("install.sh must not delete existing files")
	}
	issue, err := os.ReadFile("assets/issue-client.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(issue), "--alt-port") || !strings.Contains(string(issue), "--client-only") {
		t.Fatal("issuer must pass alt_port without replacing the server cert")
	}
}

func TestParseCertNumber(t *testing.T) {
	n, err := ParseCertNumber(" 7 ")
	if err != nil || n != 7 {
		t.Fatalf("%d %v", n, err)
	}
	cn, err := ClientCN(7)
	if err != nil || cn != "masque-client-7" {
		t.Fatalf("%s %v", cn, err)
	}
	if _, err := ParseCertNumber("x"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := ClientCN(0); err == nil {
		t.Fatal("expected reject")
	}
}
