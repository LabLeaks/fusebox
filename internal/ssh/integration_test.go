//go:build integration

package ssh

import (
	"bytes"
	"os"
	"testing"
)

func TestIntegrationConnect(t *testing.T) {
	host := os.Getenv("FUSEBOX_TEST_HOST")
	user := os.Getenv("FUSEBOX_TEST_USER")
	if host == "" || user == "" {
		t.Skip("FUSEBOX_TEST_HOST and FUSEBOX_TEST_USER required for integration tests")
	}

	client, err := Connect(host, user)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	t.Run("RunCommand", func(t *testing.T) {
		stdout, stderr, exitCode, err := client.RunCommand("echo hello")
		if err != nil {
			t.Fatalf("RunCommand: %v", err)
		}
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr: %s", exitCode, stderr)
		}
		if stdout != "hello\n" {
			t.Errorf("stdout = %q, want %q", stdout, "hello\n")
		}
	})

	t.Run("RunCommandNonZeroExit", func(t *testing.T) {
		_, _, exitCode, err := client.RunCommand("exit 42")
		if err != nil {
			t.Fatalf("RunCommand: %v", err)
		}
		if exitCode != 42 {
			t.Errorf("exit code = %d, want 42", exitCode)
		}
	})

	t.Run("RunCommandStream", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode, err := client.RunCommandStream("echo streamed", &stdout, &stderr)
		if err != nil {
			t.Fatalf("RunCommandStream: %v", err)
		}
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if stdout.String() != "streamed\n" {
			t.Errorf("stdout = %q, want %q", stdout.String(), "streamed\n")
		}
	})

	t.Run("ReverseTunnel", func(t *testing.T) {
		tunnel, err := client.ReverseTunnel(19999, 22)
		if err != nil {
			t.Fatalf("ReverseTunnel: %v", err)
		}
		defer tunnel.Close()
	})
}
