package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lableaks/fusebox/internal/sync"
)

// mockMutagenRunner records calls for verification.
type mockMutagenRunner struct {
	calls   []mockCall
	results []mockResult
	idx     int
}

type mockCall struct {
	Name string
	Args []string
}

type mockResult struct {
	Stdout string
	Stderr string
	Err    error
}

func (m *mockMutagenRunner) Run(name string, args ...string) (string, string, error) {
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	if m.idx < len(m.results) {
		r := m.results[m.idx]
		m.idx++
		return r.Stdout, r.Stderr, r.Err
	}
	return "", "", nil
}

func newTestMutagen(results ...mockResult) (*sync.MutagenManager, *mockMutagenRunner) {
	runner := &mockMutagenRunner{results: results}
	mgr := sync.NewMutagenManagerWithRunner(runner)
	return mgr, runner
}

// mockContainerManager tracks stop/remove calls.
type mockContainerManager struct {
	stopped []string
	removed []string
	stopErr error
	rmErr   error
}

func (m *mockContainerManager) Stop(projectName string) error {
	m.stopped = append(m.stopped, projectName)
	return m.stopErr
}

func (m *mockContainerManager) Remove(projectName string) error {
	m.removed = append(m.removed, projectName)
	return m.rmErr
}

func TestDown_PausesSessions(t *testing.T) {
	// Two calls: pause src, pause claude
	mgr, runner := newTestMutagen(mockResult{}, mockResult{})
	cm := &mockContainerManager{}
	var output []string

	err := Down(DownOptions{ProjectName: "myapp"}, mgr, cm, func(s string) {
		output = append(output, s)
	})
	if err != nil {
		t.Fatalf("Down error: %v", err)
	}

	// Verify pause was called for both sessions
	pauseCalls := 0
	for _, call := range runner.calls {
		if len(call.Args) >= 2 && call.Args[0] == "sync" && call.Args[1] == "pause" {
			pauseCalls++
		}
	}
	if pauseCalls != 2 {
		t.Errorf("expected 2 pause calls, got %d", pauseCalls)
	}

	// Should not stop/remove container
	if len(cm.stopped) != 0 || len(cm.removed) != 0 {
		t.Error("should not stop/remove container without --destroy")
	}

	// Check output message
	found := false
	for _, line := range output {
		if strings.Contains(line, "warm restart") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warm restart message, got: %v", output)
	}
}

func TestDown_Destroy(t *testing.T) {
	// Two calls: terminate src, terminate claude
	mgr, runner := newTestMutagen(mockResult{}, mockResult{})
	cm := &mockContainerManager{}
	var output []string

	err := Down(DownOptions{ProjectName: "myapp", Destroy: true}, mgr, cm, func(s string) {
		output = append(output, s)
	})
	if err != nil {
		t.Fatalf("Down error: %v", err)
	}

	// Verify terminate was called for both sessions
	termCalls := 0
	for _, call := range runner.calls {
		if len(call.Args) >= 2 && call.Args[0] == "sync" && call.Args[1] == "terminate" {
			termCalls++
		}
	}
	if termCalls != 2 {
		t.Errorf("expected 2 terminate calls, got %d", termCalls)
	}

	// Container should be stopped and removed
	if len(cm.stopped) != 1 || cm.stopped[0] != "myapp" {
		t.Errorf("expected stop myapp, got: %v", cm.stopped)
	}
	if len(cm.removed) != 1 || cm.removed[0] != "myapp" {
		t.Errorf("expected remove myapp, got: %v", cm.removed)
	}

	// Check destroyed message
	found := false
	for _, line := range output {
		if line == "Destroyed." {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Destroyed.' message, got: %v", output)
	}
}

func TestDown_DestroyGracefulContainerErrors(t *testing.T) {
	mgr, _ := newTestMutagen(mockResult{}, mockResult{})
	cm := &mockContainerManager{
		stopErr: fmt.Errorf("container is not running"),
		rmErr:   fmt.Errorf("No such container: fusebox-myapp"),
	}
	var output []string

	err := Down(DownOptions{ProjectName: "myapp", Destroy: true}, mgr, cm, func(s string) {
		output = append(output, s)
	})
	if err != nil {
		t.Fatalf("Down should not error on already-stopped container: %v", err)
	}

	// Should not have warnings for known-graceful errors
	for _, line := range output {
		if strings.Contains(line, "Warning") {
			t.Errorf("unexpected warning: %s", line)
		}
	}
}

func TestDown_SessionNotFoundGraceful(t *testing.T) {
	mgr, _ := newTestMutagen(
		mockResult{Stderr: "unable to locate", Err: fmt.Errorf("exit 1")},
		mockResult{Stderr: "session not found", Err: fmt.Errorf("exit 1")},
	)
	cm := &mockContainerManager{}
	var output []string

	err := Down(DownOptions{ProjectName: "myapp"}, mgr, cm, func(s string) {
		output = append(output, s)
	})
	if err != nil {
		t.Fatalf("Down should not error on missing sessions: %v", err)
	}

	for _, line := range output {
		if strings.Contains(line, "Warning") {
			t.Errorf("unexpected warning: %s", line)
		}
	}
}

func TestStopDaemon_NoPidFile(t *testing.T) {
	tmpDir := t.TempDir()
	var output []string

	stopDaemon(tmpDir, "myapp", func(s string) {
		output = append(output, s)
	})

	if len(output) != 1 || !strings.Contains(output[0], "no PID file") {
		t.Errorf("expected 'no PID file' message, got: %v", output)
	}
}

func TestStopDaemon_InvalidPidFile(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "myapp.pid"), []byte("notanumber"), 0644)

	var output []string
	stopDaemon(tmpDir, "myapp", func(s string) {
		output = append(output, s)
	})

	if len(output) != 1 || !strings.Contains(output[0], "invalid PID") {
		t.Errorf("expected 'invalid PID' message, got: %v", output)
	}
}

func TestStopDaemon_StalePid(t *testing.T) {
	tmpDir := t.TempDir()
	// PID 99999999 almost certainly doesn't exist
	os.WriteFile(filepath.Join(tmpDir, "myapp.pid"), []byte("99999999"), 0644)

	var output []string
	stopDaemon(tmpDir, "myapp", func(s string) {
		output = append(output, s)
	})

	if len(output) != 1 || !strings.Contains(output[0], "already stopped") {
		t.Errorf("expected 'already stopped' message, got: %v", output)
	}

	// PID file should be cleaned up
	if _, err := os.Stat(filepath.Join(tmpDir, "myapp.pid")); !os.IsNotExist(err) {
		t.Error("PID file should be removed after stale process")
	}
}

func TestPIDFileLifecycle(t *testing.T) {
	// Full round trip: write PID file, verify contents, read back, clean up.
	tmpDir := t.TempDir()
	pidPath := filepath.Join(tmpDir, "myapp.pid")
	pid := os.Getpid()

	// Write (as up.go does)
	err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0600)
	if err != nil {
		t.Fatalf("write PID file: %v", err)
	}

	// Read back (as down.go does)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read PID file: %v", err)
	}
	readPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse PID: %v", err)
	}
	if readPID != pid {
		t.Errorf("PID = %d, want %d", readPID, pid)
	}

	// Cleanup (as up.go defer does)
	os.Remove(pidPath)
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should not exist after removal")
	}
}

func TestDown_SessionNames(t *testing.T) {
	mgr, runner := newTestMutagen(mockResult{}, mockResult{})
	cm := &mockContainerManager{}

	_ = Down(DownOptions{ProjectName: "web-app"}, mgr, cm, func(string) {})

	var sessionNames []string
	for _, call := range runner.calls {
		if len(call.Args) >= 3 && call.Args[0] == "sync" && call.Args[1] == "pause" {
			sessionNames = append(sessionNames, call.Args[2])
		}
	}

	if len(sessionNames) != 2 {
		t.Fatalf("expected 2 session names, got %d", len(sessionNames))
	}
	if sessionNames[0] != "fusebox-src-web-app" {
		t.Errorf("src session = %q, want fusebox-src-web-app", sessionNames[0])
	}
	if sessionNames[1] != "fusebox-claude-web-app" {
		t.Errorf("claude session = %q, want fusebox-claude-web-app", sessionNames[1])
	}
}

func TestIsProcessGone(t *testing.T) {
	if !isProcessGone(fmt.Errorf("os: process already finished")) {
		t.Error("should detect 'process already finished'")
	}
	if !isProcessGone(fmt.Errorf("no such process")) {
		t.Error("should detect 'no such process'")
	}
	if isProcessGone(fmt.Errorf("permission denied")) {
		t.Error("should not match 'permission denied'")
	}
}

func TestCheckActionRunning_ParsesStatusInfo(t *testing.T) {
	// Verify that daemonStatus struct can decode ActionRunning from StatusInfo JSON.
	// This is the core of Bug #27: the status socket now returns action_running.
	jsonData := `{"project":"test","action_running":true,"last_action":{"name":"build","exit_code":0,"duration_ms":100,"timestamp":"2026-01-01T00:00:00Z"}}`

	var status daemonStatus
	if err := json.Unmarshal([]byte(jsonData), &status); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !status.ActionRunning {
		t.Error("expected ActionRunning to be true")
	}
	if status.LastAction == nil || status.LastAction.Name != "build" {
		t.Errorf("LastAction.Name = %v, want 'build'", status.LastAction)
	}
}

func TestCheckActionRunning_FalseWhenNotRunning(t *testing.T) {
	jsonData := `{"project":"test","action_running":false}`

	var status daemonStatus
	if err := json.Unmarshal([]byte(jsonData), &status); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if status.ActionRunning {
		t.Error("expected ActionRunning to be false")
	}
	if status.LastAction != nil {
		t.Error("expected LastAction to be nil when omitted")
	}
}

func TestIsSessionNotFound(t *testing.T) {
	if !isSessionNotFound(fmt.Errorf("unable to locate session")) {
		t.Error("should detect 'unable to locate'")
	}
	if !isSessionNotFound(fmt.Errorf("session not found")) {
		t.Error("should detect 'session not found'")
	}
	if isSessionNotFound(fmt.Errorf("timeout")) {
		t.Error("should not match 'timeout'")
	}
}
