package vpssetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileQRFitsSample(t *testing.T) {
	path := filepath.Join("..", "..", "..", "android", "app", "src", "main", "assets", "sample-profile.masque")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, err := ProfileQR(raw)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() < 400 || b.Dy() < 400 {
		t.Fatalf("qr size %v", b)
	}
	if _, err := ProfileQR([]byte("not a profile")); err == nil {
		t.Fatal("expected reject")
	}
	long := strings.Repeat("A", 8000)
	if _, err := ProfileQR([]byte("BEGIN\n" + long)); err == nil {
		t.Fatal("expected too large")
	}
}
