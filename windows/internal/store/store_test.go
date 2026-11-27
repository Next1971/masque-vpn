package store

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

const (
	testBundleA = `[server]
address = "vpn-a.example.com:443"
name    = "vpn-a.example.com"

[tun]
dns = "1.1.1.1"

[tls]
ca = """
-----BEGIN CERTIFICATE-----
CA-A
-----END CERTIFICATE-----
"""
cert = """
-----BEGIN CERTIFICATE-----
CLIENT-A
-----END CERTIFICATE-----
"""
key = """
-----BEGIN EC PRIVATE KEY-----
KEY-A
-----END EC PRIVATE KEY-----
"""
`

	testBundleB = `[server]
address = "vpn-b.example.com:443"
name    = "vpn-b.example.com"

[tun]
dns = "1.1.1.1"

[tls]
ca = """
-----BEGIN CERTIFICATE-----
CA-B
-----END CERTIFICATE-----
"""
cert = """
-----BEGIN CERTIFICATE-----
CLIENT-B
-----END CERTIFICATE-----
"""
key = """
-----BEGIN EC PRIVATE KEY-----
KEY-B
-----END EC PRIVATE KEY-----
"""
`
)

func testDataDir(t *testing.T) {
	t.Helper()
	t.Setenv("PROGRAMDATA", t.TempDir())
}

func writeLegacy(t *testing.T, name, pem string) string {
	t.Helper()
	dir := filepath.Join(Dir(), "certs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(pem+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportActivateUpdateList(t *testing.T) {
	testDataDir(t)

	if err := Import(testBundleA, "a.masque", "", "", ""); err != nil {
		t.Fatalf("import A: %v", err)
	}
	if !Configured() {
		t.Fatal("Configured = false after first import")
	}
	first, err := List()
	if err != nil || len(first) != 1 {
		t.Fatalf("List after A: %v %v", first, err)
	}
	if !first[0].Active || first[0].Name != "vpn-a.example.com" {
		t.Fatalf("A info wrong: %+v", first[0])
	}

	// A second server adds a profile instead of replacing.
	if err := Import(testBundleB, "b.masque", "", "", ""); err != nil {
		t.Fatalf("import B: %v", err)
	}
	all, err := List()
	if err != nil || len(all) != 2 {
		t.Fatalf("List after B: %v %v", all, err)
	}
	active, err := ActiveID()
	if err != nil {
		t.Fatal(err)
	}
	wantB := ""
	for _, info := range all {
		if info.Name == "vpn-b.example.com" {
			wantB = info.ID
		} else if info.Active {
			t.Fatal("import should activate the imported profile")
		}
	}
	if wantB == "" || active != wantB {
		t.Fatalf("active = %q, want B id %q", active, wantB)
	}

	// Re-importing A updates its entry in place and reactivates it.
	if err := Import(testBundleA, "a.masque", "", "", ""); err != nil {
		t.Fatalf("re-import A: %v", err)
	}
	all, err = List()
	if err != nil || len(all) != 2 {
		t.Fatalf("List after re-import: %v %v", all, err)
	}
	for _, info := range all {
		if info.Name == "vpn-a.example.com" && !info.Active {
			t.Fatal("re-imported A should be active")
		}
	}
}

func TestSetActiveAndDelete(t *testing.T) {
	testDataDir(t)
	if err := Import(testBundleA, "a.masque", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := Import(testBundleB, "b.masque", "", "", ""); err != nil {
		t.Fatal(err)
	}
	all, _ := List()
	var idA string
	for _, info := range all {
		if info.Name == "vpn-a.example.com" {
			idA = info.ID
		}
	}

	if err := SetActive(idA); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if active, _ := ActiveID(); active != idA {
		t.Fatalf("active = %q, want %q", active, idA)
	}
	prof, err := Load()
	if err != nil || prof.ServerName != "vpn-a.example.com" {
		t.Fatalf("Load active: %v %+v", err, prof)
	}

	if err := Delete(idA); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, _ = List()
	if len(all) != 1 || all[0].Name != "vpn-b.example.com" || !all[0].Active {
		t.Fatalf("after delete: %+v", all)
	}

	// Deleting the last profile clears the active pointer.
	if err := Delete(all[0].ID); err != nil {
		t.Fatalf("Delete last: %v", err)
	}
	if Configured() {
		t.Fatal("Configured = true with no profiles")
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load should fail with no profiles")
	}
}

func TestMigrateLegacyLayout(t *testing.T) {
	testDataDir(t)
	caPath := writeLegacy(t, "ca.crt", "-----BEGIN CERTIFICATE-----\nCA-A\n-----END CERTIFICATE-----")
	certPath := writeLegacy(t, "client.crt", "-----BEGIN CERTIFICATE-----\nCLIENT-A\n-----END CERTIFICATE-----")
	keyPath := writeLegacy(t, "client.key", "-----BEGIN EC PRIVATE KEY-----\nKEY-A\n-----END EC PRIVATE KEY-----")
	legacy := `[server]
server = "vpn-a.example.com:443"
server_name = "vpn-a.example.com"

[tls]
ca = ` + strconv.Quote(caPath) + `
cert = ` + strconv.Quote(certPath) + `
key = ` + strconv.Quote(keyPath) + `
insecure = false

[tun]
tun_name = "masque0"
mtu = 1340
dns = ["1.1.1.1"]
`
	if err := os.WriteFile(profilePath(), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	if !Configured() {
		t.Fatal("Configured = false after migration")
	}
	if _, err := os.Stat(profilePath()); !os.IsNotExist(err) {
		t.Fatal("legacy profile.toml should be removed after migration")
	}
	if _, err := os.Stat(filepath.Join(Dir(), "certs")); !os.IsNotExist(err) {
		t.Fatal("legacy certs dir should be removed after migration")
	}
	all, err := List()
	if err != nil || len(all) != 1 {
		t.Fatalf("List after migration: %v %v", all, err)
	}
	if !all[0].Active || all[0].Name != "vpn-a.example.com" {
		t.Fatalf("migrated info: %+v", all[0])
	}
	prof, err := Load()
	if err != nil || prof.ServerName != "vpn-a.example.com" {
		t.Fatalf("Load after migration: %v %+v", err, prof)
	}
	// A rerun finds nothing to migrate and stays configured.
	if err := Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if !Configured() {
		t.Fatal("Configured lost after second Migrate")
	}
}
