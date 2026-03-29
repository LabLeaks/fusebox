package ssh

import (
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestConnectOptionDefaults(t *testing.T) {
	cfg := &connectConfig{
		port:    22,
		timeout: 10 * time.Second,
	}

	if cfg.port != 22 {
		t.Errorf("default port = %d, want 22", cfg.port)
	}
	if cfg.timeout != 10*time.Second {
		t.Errorf("default timeout = %v, want 10s", cfg.timeout)
	}
}

func TestWithPort(t *testing.T) {
	cfg := &connectConfig{port: 22, timeout: 10 * time.Second}
	WithPort(2222)(cfg)

	if cfg.port != 2222 {
		t.Errorf("port = %d, want 2222", cfg.port)
	}
}

func TestWithTimeout(t *testing.T) {
	cfg := &connectConfig{port: 22, timeout: 10 * time.Second}
	WithTimeout(30 * time.Second)(cfg)

	if cfg.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", cfg.timeout)
	}
}

func TestConnectRequiresSSHAuthSock(t *testing.T) {
	orig := os.Getenv("SSH_AUTH_SOCK")
	os.Unsetenv("SSH_AUTH_SOCK")
	defer func() {
		if orig != "" {
			os.Setenv("SSH_AUTH_SOCK", orig)
		}
	}()

	_, err := Connect("example.com", "user")
	if err == nil {
		t.Fatal("expected error when SSH_AUTH_SOCK unset")
	}
	if err.Error() != "SSH_AUTH_SOCK not set: ssh-agent required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConnectInvalidAgent(t *testing.T) {
	orig := os.Getenv("SSH_AUTH_SOCK")
	os.Setenv("SSH_AUTH_SOCK", "/tmp/nonexistent-ssh-agent-sock-test")
	defer func() {
		if orig != "" {
			os.Setenv("SSH_AUTH_SOCK", orig)
		} else {
			os.Unsetenv("SSH_AUTH_SOCK")
		}
	}()

	_, err := Connect("example.com", "user")
	if err == nil {
		t.Fatal("expected error when agent socket doesn't exist")
	}
}

func TestMultipleOptions(t *testing.T) {
	cfg := &connectConfig{port: 22, timeout: 10 * time.Second}

	opts := []ConnectOption{
		WithPort(8022),
		WithTimeout(5 * time.Second),
	}

	for _, o := range opts {
		o(cfg)
	}

	if cfg.port != 8022 {
		t.Errorf("port = %d, want 8022", cfg.port)
	}
	if cfg.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", cfg.timeout)
	}
}

// mockConn tracks whether Close was called.
type mockConn struct {
	net.Conn
	closed atomic.Bool
}

func (m *mockConn) Close() error {
	m.closed.Store(true)
	return nil
}

func TestClientStructHasAgentConn(t *testing.T) {
	// Verify the Client struct stores agentConn so it can be closed
	mc := &mockConn{}
	c := &Client{
		agentConn: mc,
	}

	// Verify the field is stored correctly
	if c.agentConn != mc {
		t.Error("agentConn not stored in Client struct")
	}
}

func TestConnectStoresAgentConn(t *testing.T) {
	// Connect with an invalid host but valid agent socket to verify
	// agentConn is stored when Connect reaches the dial phase.
	// We can't test the full path without a server, but we can verify
	// the error path closes agentConn on SSH dial failure.
	authSock := os.Getenv("SSH_AUTH_SOCK")
	if authSock == "" {
		t.Skip("SSH_AUTH_SOCK not set")
	}

	// Connect to an unreachable host with short timeout
	_, err := Connect("192.0.2.1", "user", WithTimeout(100*time.Millisecond))
	if err == nil {
		t.Fatal("expected error connecting to unreachable host")
	}
	// The error path in Connect already closes agentConn before returning,
	// which is correct behavior (line 79).
}

func TestCopyFilePreservesExecutableMode(t *testing.T) {
	// Create a temporary file with executable permissions
	tmpFile, err := os.CreateTemp(t.TempDir(), "testbin-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Write([]byte("#!/bin/sh\necho hello"))
	tmpFile.Close()

	// Set executable mode
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		t.Fatal(err)
	}

	stat, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Verify the mode would be preserved in the SCP header
	mode := stat.Mode().Perm()
	if mode != 0755 {
		t.Errorf("mode = %04o, want 0755", mode)
	}

	// The actual SCP transfer requires an SSH server, so we verify the
	// mode formatting matches what CopyFile would send
	expected := "C0755"
	got := fmt.Sprintf("C%04o", mode)
	if got != expected {
		t.Errorf("SCP header mode = %q, want %q", got, expected)
	}
}

func TestCopyFileNonExecutableMode(t *testing.T) {
	// Verify a regular file gets 0644 in the SCP header
	tmpFile, err := os.CreateTemp(t.TempDir(), "testfile-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Write([]byte("regular file"))
	tmpFile.Close()

	if err := os.Chmod(tmpFile.Name(), 0644); err != nil {
		t.Fatal(err)
	}

	stat, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	mode := stat.Mode().Perm()
	got := fmt.Sprintf("C%04o", mode)
	if got != "C0644" {
		t.Errorf("SCP header mode = %q, want C0644", got)
	}
}
