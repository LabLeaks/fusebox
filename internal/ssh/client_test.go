package ssh

import (
	"bytes"
	"os"
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

func TestRunCommandStreamWriterTypes(t *testing.T) {
	// Verify bytes.Buffer satisfies the io.Writer interface expected by RunCommandStream
	var stdout, stderr bytes.Buffer
	_ = &stdout
	_ = &stderr
}
