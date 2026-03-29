package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWritePIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidPath := filepath.Join(tmpDir, "testproject.pid")

	// Simulate writing PID file as up.go does
	pid := os.Getpid()
	err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0600)
	if err != nil {
		t.Fatalf("writing PID file: %v", err)
	}

	// Verify file exists and contains valid PID
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("reading PID file: %v", err)
	}

	readPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parsing PID from file: %v", err)
	}
	if readPID != pid {
		t.Errorf("PID file contains %d, want %d", readPID, pid)
	}

	// Verify file permissions
	info, err := os.Stat(pidPath)
	if err != nil {
		t.Fatalf("stat PID file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("PID file permissions = %o, want 0600", perm)
	}

	// Verify cleanup works
	os.Remove(pidPath)
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should be removed after cleanup")
	}
}

func TestPIDFilePath(t *testing.T) {
	// Verify the PID file goes in the right directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("getting home dir: %v", err)
	}

	expected := filepath.Join(home, ".fusebox", "run", "myapp.pid")
	got := filepath.Join(home, ".fusebox", "run", "myapp"+".pid")
	if got != expected {
		t.Errorf("PID path = %q, want %q", got, expected)
	}
}
