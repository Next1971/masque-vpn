package vpssetup

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Auth is SSH password and/or private key (PEM). Key wins if both are set for PublicKeys then password as fallback.
type Auth struct {
	User        string
	Password    string
	KeyPEM      []byte
	KeyPassword string
}

// UnknownHostError is returned when the server key is not in the known_hosts file.
type UnknownHostError struct {
	Host        string
	Fingerprint string
	Key         ssh.PublicKey
}

func (e *UnknownHostError) Error() string {
	return fmt.Sprintf("unknown SSH host %s fingerprint %s", e.Host, e.Fingerprint)
}

// MismatchedHostKeyError is a MITM-style warning: the key changed.
type MismatchedHostKeyError struct {
	Host string
}

func (e *MismatchedHostKeyError) Error() string {
	return fmt.Sprintf("SSH host key for %s does not match known_hosts", e.Host)
}

// Client is a root (or sudo-capable) SSH session to the VPS.
type Client struct {
	c *ssh.Client
}

// Dial connects with a 20s timeout. knownHostsPath is a known_hosts file (created if missing).
func Dial(host string, sshPort int, auth Auth, knownHostsPath string) (*Client, error) {
	if err := ValidateHost(host); err != nil {
		return nil, err
	}
	if sshPort < 1 || sshPort > 65535 {
		return nil, fmt.Errorf("invalid SSH port")
	}
	user := strings.TrimSpace(auth.User)
	if user == "" {
		user = "root"
	}
	methods, err := authMethods(auth)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(knownHostsPath), 0o700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(knownHostsPath); errors.Is(err, os.ErrNotExist) {
		if werr := os.WriteFile(knownHostsPath, []byte{}, 0o600); werr != nil {
			return nil, werr
		}
	}
	hostCB, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", sshPort))
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            methods,
		HostKeyCallback: wrapKnownHosts(hostCB, addr),
		Timeout:         20 * time.Second,
	}
	c, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return &Client{c: c}, nil
}

func wrapKnownHosts(inner ssh.HostKeyCallback, dialAddr string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := inner(hostname, remote, key)
		if err == nil {
			return nil
		}
		var ke *knownhosts.KeyError
		if !errors.As(err, &ke) {
			return err
		}
		fp := ssh.FingerprintSHA256(key)
		if len(ke.Want) == 0 {
			return &UnknownHostError{Host: dialAddr, Fingerprint: fp, Key: key}
		}
		return &MismatchedHostKeyError{Host: dialAddr}
	}
}

// AppendKnownHost writes a known_hosts line for key at hostPort ("1.2.3.4:22").
func AppendKnownHost(knownHostsPath, hostPort string, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(knownHostsPath), 0o700); err != nil {
		return err
	}
	n := knownhosts.Normalize(hostPort)
	addrs := []string{n}
	if n != hostPort {
		addrs = append(addrs, hostPort)
	}
	line := knownhosts.Line(addrs, key)
	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}

func authMethods(a Auth) ([]ssh.AuthMethod, error) {
	var out []ssh.AuthMethod
	if len(a.KeyPEM) > 0 {
		var signer ssh.Signer
		var err error
		if a.KeyPassword != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(a.KeyPEM, []byte(a.KeyPassword))
		} else {
			signer, err = ssh.ParsePrivateKey(a.KeyPEM)
		}
		if err != nil {
			return nil, fmt.Errorf("parse SSH key: %w", err)
		}
		out = append(out, ssh.PublicKeys(signer))
	}
	if a.Password != "" {
		out = append(out, ssh.Password(a.Password))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provide a password or a private key")
	}
	return out, nil
}

func (c *Client) Close() error {
	if c == nil || c.c == nil {
		return nil
	}
	return c.c.Close()
}

// Run executes cmd on the remote host. stdin may be nil.
func (c *Client) Run(timeout time.Duration, cmd string, stdin io.Reader) (string, error) {
	b, err := c.RunBytes(timeout, cmd, stdin)
	return strings.TrimSpace(string(b)), err
}

// RunBytes is Run but returns raw stdout (for tar).
func (c *Client) RunBytes(timeout time.Duration, cmd string, stdin io.Reader) ([]byte, error) {
	s, err := c.c.NewSession()
	if err != nil {
		return nil, err
	}
	defer s.Close()
	var stdout, stderr bytes.Buffer
	s.Stdout = &stdout
	s.Stderr = &stderr
	s.Stdin = stdin
	done := make(chan error, 1)
	go func() { done <- s.Run(cmd) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			mix := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
			if mix != "" {
				return stdout.Bytes(), fmt.Errorf("%w: %s", err, mix)
			}
			return stdout.Bytes(), err
		}
		return stdout.Bytes(), nil
	case <-timer.C:
		_ = s.Close()
		return nil, fmt.Errorf("remote command timed out after %s", timeout)
	}
}

// Upload writes local bytes to remotePath (mode 0644 or 0755 if executable).
func (c *Client) Upload(remotePath string, data []byte, mode os.FileMode) error {
	s, err := c.c.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()
	s.Stdin = bytes.NewReader(data)
	cmd := fmt.Sprintf("umask 077; cat > %s && chmod %o %s",
		shellSingleQuote(remotePath), mode.Perm(), shellSingleQuote(remotePath))
	var stderr bytes.Buffer
	s.Stderr = &stderr
	if err := s.Run(cmd); err != nil {
		return fmt.Errorf("upload %s: %w (%s)", remotePath, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
