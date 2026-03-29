package sync

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// mockRunner records calls and returns canned responses.
type mockRunner struct {
	calls   []mockCall
	results []mockResult
	callIdx int
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

func (m *mockRunner) Run(name string, args ...string) (string, string, error) {
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	if m.callIdx < len(m.results) {
		r := m.results[m.callIdx]
		m.callIdx++
		return r.Stdout, r.Stderr, r.Err
	}
	return "", "", nil
}

func TestSrcSessionName(t *testing.T) {
	got := SrcSessionName("myproject")
	want := "fusebox-src-myproject"
	if got != want {
		t.Errorf("SrcSessionName = %q, want %q", got, want)
	}
}

func TestClaudeSessionName(t *testing.T) {
	got := ClaudeSessionName("myproject")
	want := "fusebox-claude-myproject"
	if got != want {
		t.Errorf("ClaudeSessionName = %q, want %q", got, want)
	}
}

func TestMergeIgnores_DefaultsIncluded(t *testing.T) {
	result := mergeIgnores(nil)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != ".git" || result[1] != "fusebox.yaml" {
		t.Errorf("defaults = %v, want [.git fusebox.yaml]", result)
	}
}

func TestMergeIgnores_UserAdded(t *testing.T) {
	result := mergeIgnores([]string{"node_modules", "dist"})
	want := []string{".git", "fusebox.yaml", "node_modules", "dist"}
	if len(result) != len(want) {
		t.Fatalf("len = %d, want %d", len(result), len(want))
	}
	for i, v := range want {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestMergeIgnores_Deduplication(t *testing.T) {
	result := mergeIgnores([]string{".git", "node_modules", "fusebox.yaml"})
	want := []string{".git", "fusebox.yaml", "node_modules"}
	if len(result) != len(want) {
		t.Fatalf("len = %d, want %d; got %v", len(result), len(want), result)
	}
	for i, v := range want {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestBuildCreateArgs(t *testing.T) {
	args := buildCreateArgs(
		"fusebox-src-myapp",
		"/home/user/myapp",
		"root",
		"10.0.0.1",
		"/workspace/myapp",
		[]string{"node_modules"},
	)

	got := strings.Join(args, " ")
	want := "sync create /home/user/myapp root@10.0.0.1:/workspace/myapp --name fusebox-src-myapp --ignore .git --ignore fusebox.yaml --ignore node_modules"
	if got != want {
		t.Errorf("args =\n  %s\nwant:\n  %s", got, want)
	}
}

func TestBuildCreateArgs_NoUserIgnores(t *testing.T) {
	args := buildCreateArgs(
		"fusebox-src-bare",
		"/src",
		"deploy",
		"host",
		"/dst",
		nil,
	)

	got := strings.Join(args, " ")
	want := "sync create /src deploy@host:/dst --name fusebox-src-bare --ignore .git --ignore fusebox.yaml"
	if got != want {
		t.Errorf("args =\n  %s\nwant:\n  %s", got, want)
	}
}

func TestCreate_Success(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{{Stdout: "", Stderr: "", Err: nil}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.Create("fusebox-src-test", "/local", "user", "host", "/remote", nil)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.Name != "mutagen" {
		t.Errorf("binary = %q, want mutagen", call.Name)
	}
	if call.Args[0] != "sync" || call.Args[1] != "create" {
		t.Errorf("subcommand = %v, want [sync create ...]", call.Args[:2])
	}
}

func TestCreate_Error(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{{Stderr: "session already exists", Err: fmt.Errorf("exit 1")}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.Create("fusebox-src-test", "/local", "user", "host", "/remote", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "session already exists") {
		t.Errorf("error = %q, want to contain stderr", err.Error())
	}
}

func TestResume_CommandArgs(t *testing.T) {
	runner := &mockRunner{results: []mockResult{{}}}
	mgr := NewMutagenManagerWithRunner(runner)

	_ = mgr.Resume("fusebox-src-test")

	call := runner.calls[0]
	want := "sync resume fusebox-src-test"
	got := strings.Join(call.Args, " ")
	if got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestPause_CommandArgs(t *testing.T) {
	runner := &mockRunner{results: []mockResult{{}}}
	mgr := NewMutagenManagerWithRunner(runner)

	_ = mgr.Pause("fusebox-src-test")

	call := runner.calls[0]
	want := "sync pause fusebox-src-test"
	got := strings.Join(call.Args, " ")
	if got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestTerminate_CommandArgs(t *testing.T) {
	runner := &mockRunner{results: []mockResult{{}}}
	mgr := NewMutagenManagerWithRunner(runner)

	_ = mgr.Terminate("fusebox-src-test")

	call := runner.calls[0]
	want := "sync terminate fusebox-src-test"
	got := strings.Join(call.Args, " ")
	if got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestSessionStatus_ParsesOutput(t *testing.T) {
	mutagenOutput := `Session: fusebox-src-myapp
Identifier: abc123
Alpha:
	URL: /home/user/myapp
Beta:
	URL: root@10.0.0.1:/workspace/myapp
Status: Watching for changes
`
	runner := &mockRunner{
		results: []mockResult{{Stdout: mutagenOutput}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	status, err := mgr.SessionStatus("fusebox-src-myapp")
	if err != nil {
		t.Fatalf("SessionStatus error: %v", err)
	}
	if status != "Watching for changes" {
		t.Errorf("status = %q, want %q", status, "Watching for changes")
	}
}

func TestSessionStatus_Scanning(t *testing.T) {
	mutagenOutput := `Session: fusebox-src-myapp
Status: Scanning files
`
	runner := &mockRunner{
		results: []mockResult{{Stdout: mutagenOutput}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	status, err := mgr.SessionStatus("fusebox-src-myapp")
	if err != nil {
		t.Fatalf("SessionStatus error: %v", err)
	}
	if status != "Scanning files" {
		t.Errorf("status = %q, want %q", status, "Scanning files")
	}
}

func TestSessionStatus_Unknown(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{{Stdout: "no status line here\n"}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	status, err := mgr.SessionStatus("fusebox-src-myapp")
	if err != nil {
		t.Fatalf("SessionStatus error: %v", err)
	}
	if status != "Unknown" {
		t.Errorf("status = %q, want %q", status, "Unknown")
	}
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"watching", "Status: Watching for changes\n", "Watching for changes"},
		{"scanning", "  Status: Scanning files\n", "Scanning files"},
		{"staging", "Status: Staging files\n", "Staging files"},
		{"empty", "", "Unknown"},
		{"no status", "Identifier: abc\nAlpha: /foo\n", "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStatus(tt.output)
			if got != tt.want {
				t.Errorf("parseStatus = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWaitForSync_ImmediateSuccess(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stdout: "Status: Watching for changes\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.WaitForSync("fusebox-src-test", 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForSync error: %v", err)
	}
}

func TestWaitForSync_EventualSuccess(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stdout: "Status: Scanning files\n"},
			{Stdout: "Status: Staging files\n"},
			{Stdout: "Status: Watching for changes\n"},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.WaitForSync("fusebox-src-test", 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForSync error: %v", err)
	}
	if len(runner.calls) != 3 {
		t.Errorf("expected 3 polls, got %d", len(runner.calls))
	}
}

func TestWaitForSync_Timeout(t *testing.T) {
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
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.WaitForSync("fusebox-src-test", 1*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "sync timeout") {
		t.Errorf("error = %q, want to contain 'sync timeout'", err.Error())
	}
}

func TestWaitForSync_PollError(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{
			{Stderr: "session not found", Err: fmt.Errorf("exit 1")},
		},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.WaitForSync("fusebox-src-test", 5*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "polling sync status") {
		t.Errorf("error = %q, want to contain 'polling sync status'", err.Error())
	}
}

func TestNewMutagenManager_BinaryNotFound(t *testing.T) {
	// Clear PATH to ensure mutagen won't be found
	t.Setenv("PATH", "/nonexistent")

	_, err := NewMutagenManager()
	if err == nil {
		t.Fatal("expected error when mutagen not on PATH")
	}
	if !strings.Contains(err.Error(), "mutagen binary not found") {
		t.Errorf("error = %q, want to contain 'mutagen binary not found'", err.Error())
	}
}

func TestResume_Error(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{{Stderr: "session not found", Err: fmt.Errorf("exit 1")}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.Resume("fusebox-src-test")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("error = %q, want to contain 'session not found'", err.Error())
	}
}

func TestPause_Error(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{{Stderr: "session not found", Err: fmt.Errorf("exit 1")}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.Pause("fusebox-src-test")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("error = %q, want to contain 'session not found'", err.Error())
	}
}

func TestTerminate_Error(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{{Stderr: "session not found", Err: fmt.Errorf("exit 1")}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	err := mgr.Terminate("fusebox-src-test")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("error = %q, want to contain 'session not found'", err.Error())
	}
}

func TestSessionStatus_Error(t *testing.T) {
	runner := &mockRunner{
		results: []mockResult{{Stderr: "no such session", Err: fmt.Errorf("exit 1")}},
	}
	mgr := NewMutagenManagerWithRunner(runner)

	_, err := mgr.SessionStatus("fusebox-src-test")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no such session") {
		t.Errorf("error = %q, want to contain 'no such session'", err.Error())
	}
}
