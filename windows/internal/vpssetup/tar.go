package vpssetup

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func extractTar(raw []byte, dest string) error {
	tr := tar.NewReader(bytes.NewReader(raw))
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	destAbs = filepath.Clean(destAbs)
	prefix := destAbs + string(os.PathSeparator)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := tarSafePath(destAbs, hdr.Name)
		if err != nil {
			return err
		}
		if target != destAbs && !strings.HasPrefix(target, prefix) {
			return fmt.Errorf("refusing tar path %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := hdr.FileInfo().Mode()
			if mode == 0 {
				mode = 0o644
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		default:
			// skip other types
		}
	}
}

// tarSafePath maps a tar entry to destAbs. It rejects absolute names and ".."
// so extraction cannot leave destAbs (zip-slip).
func tarSafePath(destAbs, entry string) (string, error) {
	n := strings.ReplaceAll(entry, "\\", "/")
	if strings.HasPrefix(n, "/") {
		return "", fmt.Errorf("refusing tar path %q", entry)
	}
	for _, p := range strings.Split(n, "/") {
		if p == ".." {
			return "", fmt.Errorf("refusing tar path %q", entry)
		}
	}
	n = path.Clean("/" + n)
	n = strings.TrimPrefix(n, "/")
	if n == "." || n == "" {
		return destAbs, nil
	}
	for _, p := range strings.Split(n, "/") {
		if p == ".." || p == "" {
			return "", fmt.Errorf("refusing tar path %q", entry)
		}
	}
	target := filepath.Join(append([]string{destAbs}, strings.Split(n, "/")...)...)
	target = filepath.Clean(target)
	return target, nil
}
