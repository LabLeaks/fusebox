//go:build integration

package server

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var testBinary string

func TestMain(m *testing.M) {
	// Build the binary to a temp location
	tmp, err := os.CreateTemp("", "work-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp file: %v\n", err)
		os.Exit(1)
	}
	tmp.Close()
	testBinary = tmp.Name()

	cmd := exec.Command("go", "build", "-o", testBinary, "../../..")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build test binary: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	os.Remove(testBinary)
	os.Exit(code)
}

// run executes the test binary with the given args and returns stdout.
func run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(testBinary, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("command %v failed: %s\nstderr: %s", args, err, string(ee.Stderr))
		}
		t.Fatalf("command %v failed: %s", args, err)
	}
	return string(out)
}

// runExpectFail executes the test binary expecting a non-zero exit.
func runExpectFail(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(testBinary, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure for %v, got success: %s", args, string(out))
	}
	return string(out)
}

// testPrefix generates a unique session name prefix for this test.
func testPrefix(t *testing.T) string {
	return fmt.Sprintf("_test_ws_%d", os.Getpid())
}

func cleanupSessions(t *testing.T, prefix string) {
	t.Helper()
	// Kill any sessions with our test prefix
	out, _ := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(name, prefix) {
			exec.Command("tmux", "kill-session", "-t", name).Run()
		}
	}
}

func TestIntegration_Help(t *testing.T) {
	out := run(t, "help")
	if !strings.Contains(out, "Claude Code session manager") {
		t.Errorf("help output missing expected text: %s", out)
	}
	if !strings.Contains(out, "work list") {
		t.Errorf("help output missing server commands: %s", out)
	}
}

func TestIntegration_Dirs(t *testing.T) {
	out := run(t, "dirs")
	var dirs []string
	if err := json.Unmarshal([]byte(out), &dirs); err != nil {
		t.Fatalf("dirs output is not valid JSON: %v\noutput: %s", err, out)
	}
}

func TestIntegration_List(t *testing.T) {
	out := run(t, "list")
	var sessions []json.RawMessage
	if err := json.Unmarshal([]byte(out), &sessions); err != nil {
		t.Fatalf("list output is not valid JSON array: %v\noutput: %s", err, out)
	}
}

func TestIntegration_Activity(t *testing.T) {
	out := run(t, "activity")
	var activity map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &activity); err != nil {
		t.Fatalf("activity output is not valid JSON: %v\noutput: %s", err, out)
	}
}

func TestIntegration_CreateAndStop(t *testing.T) {
	prefix := testPrefix(t)
	name := prefix + "_create"
	t.Cleanup(func() { cleanupSessions(t, prefix) })

	dir := t.TempDir()

	out := run(t, "create", name, dir)
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("create output is not valid JSON: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("expected ok=true, got %v", result)
	}
	if result["name"] != name {
		t.Errorf("expected name=%s, got %v", name, result["name"])
	}

	// Session should appear in list
	listOut := run(t, "list")
	if !strings.Contains(listOut, name) {
		t.Errorf("created session %s not found in list: %s", name, listOut)
	}

	// Stop it
	stopOut := run(t, "stop", name)
	var stopResult map[string]any
	if err := json.Unmarshal([]byte(stopOut), &stopResult); err != nil {
		t.Fatalf("stop output is not valid JSON: %v", err)
	}
	if stopResult["ok"] != true {
		t.Errorf("expected stop ok=true, got %v", stopResult)
	}
}

func TestIntegration_CreateDuplicateFails(t *testing.T) {
	prefix := testPrefix(t)
	name := prefix + "_dup"
	t.Cleanup(func() { cleanupSessions(t, prefix) })

	dir := t.TempDir()
	run(t, "create", name, dir)

	// Second create should fail
	out := runExpectFail(t, "create", name, dir)
	if !strings.Contains(out, "already exists") {
		t.Errorf("expected 'already exists' error, got: %s", out)
	}
}

func TestIntegration_CreateBadDirFails(t *testing.T) {
	out := runExpectFail(t, "create", "test_bad_dir", "/nonexistent/path/xyz")
	if !strings.Contains(out, "directory not found") {
		t.Errorf("expected 'directory not found' error, got: %s", out)
	}
}

func TestIntegration_StopNonexistentFails(t *testing.T) {
	out := runExpectFail(t, "stop", "nonexistent_session_xyz")
	if !strings.Contains(out, "session not found") {
		t.Errorf("expected 'session not found' error, got: %s", out)
	}
}

func TestIntegration_Preview(t *testing.T) {
	prefix := testPrefix(t)
	name := prefix + "_preview"
	t.Cleanup(func() { cleanupSessions(t, prefix) })

	dir := t.TempDir()
	run(t, "create", name, dir)

	// Preview should return something (even if just blank lines)
	out := run(t, "preview", name)
	_ = out // just verify it doesn't error
}

func TestIntegration_FixMouse(t *testing.T) {
	out := run(t, "fix-mouse")
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("fix-mouse output is not valid JSON: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("expected ok=true, got %v", result)
	}
}
