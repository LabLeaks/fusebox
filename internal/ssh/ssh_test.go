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

func TestClient_AttachPaneCmd(t *testing.T) {
	c := NewClient("test-server", "testuser")
	cmd := c.AttachPaneCmd("my-session", 2)

	args := cmd.Args
	if args[0] != "ssh" || args[1] != "-t" {
		t.Errorf("expected ssh -t, got %v", args[:2])
	}
	// Last arg is the shell command
	shellCmd := args[len(args)-1]
	if shellCmd != "tmux select-pane -t my-session:0.2 && tmux attach-session -t my-session" {
		t.Errorf("unexpected shell command: %s", shellCmd)
	}
}

// --- LocalRunner ---

func TestNewLocalRunner(t *testing.T) {
	r := NewLocalRunner("/usr/local/bin/fusebox")
	if r.ServerPath != "/usr/local/bin/fusebox" {
		t.Errorf("expected server path, got %s", r.ServerPath)
	}
}

func TestLocalRunner_Run(t *testing.T) {
	r := NewLocalRunner("")
	out, err := r.Run("echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", string(out))
	}
}

func TestLocalRunner_RunError(t *testing.T) {
	r := NewLocalRunner("")
	_, err := r.Run("exit 1")
	if err == nil {
		t.Error("expected error for exit 1")
	}
}

func TestLocalRunner_AttachCmd(t *testing.T) {
	r := NewLocalRunner("")
	cmd := r.AttachCmd("my-session")
	args := cmd.Args
	expected := []string{"tmux", "attach-session", "-t", "my-session"}

	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, arg := range expected {
		if args[i] != arg {
			t.Errorf("arg[%d]: expected %q, got %q", i, arg, args[i])
		}
	}
}

func TestLocalRunner_AttachPaneCmd(t *testing.T) {
	r := NewLocalRunner("")
	cmd := r.AttachPaneCmd("my-session", 1)
	args := cmd.Args
	// sh -c "tmux select-pane ... && tmux attach-session ..."
	if args[0] != "sh" || args[1] != "-c" {
		t.Errorf("expected sh -c, got %v", args[:2])
	}
	shellCmd := args[2]
	if shellCmd != "tmux select-pane -t my-session:0.1 && tmux attach-session -t my-session" {
		t.Errorf("unexpected shell command: %s", shellCmd)
	}
}
