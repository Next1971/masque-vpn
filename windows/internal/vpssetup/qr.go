package vpssetup

import (
	"fmt"
	"image"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// ProfileQR draws one QR code of a profile.masque file.
// The payload is the file text, including the private key.
// Medium error correction is used when it fits; Low is the fallback.
func ProfileQR(profile []byte) (image.Image, error) {
	text := strings.TrimSpace(string(profile))
	if text == "" {
		return nil, fmt.Errorf("empty profile")
	}
	if !strings.Contains(text, "BEGIN") {
		return nil, fmt.Errorf("profile has no certificate")
	}
	q, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		q, err = qrcode.New(text, qrcode.Low)
	}
	if err != nil {
		return nil, fmt.Errorf("profile is too large for one QR code (%d bytes); use the saved file", len(text))
	}
	return q.Image(512), nil
}
