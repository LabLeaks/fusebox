package sync

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestClassifyStatus(t *testing.T) {
	tests := []struct {
		status string
		want   SyncState
	}{
		{"Watching for changes", StateWatching},
		{"Scanning files", StateSyncing},
		{"Staging files", StateSyncing},
		{"Reconciling", StateSyncing},
		{"Transitioning", StateSyncing},
		{"Paused", StatePaused},
		{"Unknown", StateUnknown},
		{"Something unexpected", StateSyncing},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := classifyStatus(tt.status)
			if got != tt.want {
				t.Errorf("classifyStatus(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestSyncWaiter_State(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stdout: "Status: Watching for changes\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)
	waiter := NewSyncWaiter(mgr)

	state, err := waiter.State("fusebox-src-test")
	if err != nil {
		t.Fatalf("State error: %v", err)
	}
	if state != StateWatching {
		t.Errorf("state = %q, want %q", state, StateWatching)
	}
}

func TestSyncWaiter_State_Syncing(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stdout: "Status: Scanning files\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)
	waiter := NewSyncWaiter(mgr)

	state, err := waiter.State("fusebox-src-test")
	if err != nil {
		t.Fatalf("State error: %v", err)
	}
	if state != StateSyncing {
		t.Errorf("state = %q, want %q", state, StateSyncing)
	}
}

func TestSyncWaiter_State_Error(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stderr: "session not found", Err: fmt.Errorf("exit 1")},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)
	waiter := NewSyncWaiter(mgr)

	state, err := waiter.State("fusebox-src-test")
	if err == nil {
		t.Fatal("expected error")
	}
	if state != StateError {
		t.Errorf("state = %q, want %q", state, StateError)
	}
}

func TestSyncWaiter_WaitForWatching_Immediate(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stdout: "Status: Watching for changes\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)
	waiter := NewSyncWaiter(mgr)

	state, err := waiter.WaitForWatching("fusebox-src-test", 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForWatching error: %v", err)
	}
	if state != StateWatching {
		t.Errorf("state = %q, want %q", state, StateWatching)
	}
}

func TestSyncWaiter_WaitForWatching_Timeout(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			// Extra for the State call after timeout
			{Stdout: "Status: Scanning files\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)
	waiter := NewSyncWaiter(mgr)

	state, err := waiter.WaitForWatching("fusebox-src-test", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if state != StateSyncing {
		t.Errorf("state = %q, want %q", state, StateSyncing)
	}
}

// --- WaitForSyncWithLog tests ---

func TestWaitForSyncWithLog_AlreadyWatching(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stdout: "Status: Watching for changes\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)
	waiter := NewSyncWaiter(mgr)

	var buf bytes.Buffer
	err := WaitForSyncWithLog(waiter, "fusebox-src-test", 5*time.Second, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not print anything if already watching.
	if buf.Len() != 0 {
		t.Errorf("expected no log output, got %q", buf.String())
	}
}

func TestWaitForSyncWithLog_WaitsAndSucceeds(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			// First State() call — syncing
			{Stdout: "Status: Scanning files\n"},
			// WaitForWatching polls
			{Stdout: "Status: Watching for changes\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)
	waiter := NewSyncWaiter(mgr)

	var buf bytes.Buffer
	err := WaitForSyncWithLog(waiter, "fusebox-src-test", 5*time.Second, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[sync] waiting for sync to complete...") {
		t.Errorf("missing waiting message in %q", output)
	}
	if !strings.Contains(output, "[sync] synced, proceeding") {
		t.Errorf("missing synced message in %q", output)
	}
}

func TestWaitForSyncWithLog_TimeoutProceedsAnyway(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			// First State() call — syncing
			{Stdout: "Status: Scanning files\n"},
			// WaitForWatching polls (will timeout)
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Scanning files\n"},
			// State() call after timeout in WaitForWatching
			{Stdout: "Status: Scanning files\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)
	waiter := NewSyncWaiter(mgr)

	var buf bytes.Buffer
	err := WaitForSyncWithLog(waiter, "fusebox-src-test", 1*time.Millisecond, &buf)
	if err != nil {
		t.Fatalf("should not error on timeout (proceeds anyway): %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[sync] warning:") {
		t.Errorf("missing warning message in %q", output)
	}
	if !strings.Contains(output, "proceeding anyway") {
		t.Errorf("missing 'proceeding anyway' in %q", output)
	}
}
