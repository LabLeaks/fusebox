package ssh

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("test-server", "testuser")
	if c.Host != "test-server" {
		t.Errorf("expected host test-server, got %s", c.Host)
	}
	if c.User != "testuser" {
		t.Errorf("expected user testuser, got %s", c.User)
	}
}

func TestClient_target(t *testing.T) {
	c := NewClient("myhost", "myuser")
	if c.target() != "myuser@myhost" {
		t.Errorf("expected myuser@myhost, got %s", c.target())
	}
}

func TestClient_AttachCmd(t *testing.T) {
	c := NewClient("test-server", "testuser")
	cmd := c.AttachCmd("my-session")

	args := cmd.Args
	expected := []string{"ssh", "-t", "testuser@test-server", "tmux", "attach-session", "-t", "my-session"}

	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, arg := range expected {
		if args[i] != arg {
			t.Errorf("arg[%d]: expected %q, got %q", i, arg, args[i])
		}
	}
}
