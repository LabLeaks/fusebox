package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestEncodePath_Standard(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"macOS project", "/Users/gk/work/project", "-Users-gk-work-project"},
		{"linux home", "/home/user/code/myapp", "-home-user-code-myapp"},
		{"deep nesting", "/Users/gk/work/lableaks/projects/fusebox", "-Users-gk-work-lableaks-projects-fusebox"},
		{"root", "/", "-"},
		{"single dir", "/tmp", "-tmp"},
		{"trailing slash stripped by caller", "/Users/gk/work/project/", "-Users-gk-work-project-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodePath(tt.path)
			if got != tt.want {
				t.Errorf("EncodePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestEncodePath_NoSlashes(t *testing.T) {
	got := EncodePath("relative-path")
	if got != "relative-path" {
		t.Errorf("EncodePath(relative) = %q, want %q", got, "relative-path")
	}
}

func TestTransformSettings_KeepsAllowedFields(t *testing.T) {
	input := `{
		"alwaysThinkingEnabled": true,
		"permissions": {"allow": ["Bash"]},
		"env": {"FOO": "bar"},
		"skipDangerousModePermissionPrompt": false,
		"hooks": {"preCommit": "lint"},
		"statusLine": "active",
		"feedbackSurveyState": {"shown": true}
	}`

	out, err := TransformSettings([]byte(input))
	if err != nil {
		t.Fatalf("TransformSettings error: %v", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Kept fields
	for _, key := range []string{"alwaysThinkingEnabled", "permissions", "env", "skipDangerousModePermissionPrompt"} {
		if _, ok := result[key]; !ok {
			t.Errorf("expected key %q to be present", key)
		}
	}

	// Dropped fields
	for _, key := range []string{"hooks", "statusLine", "feedbackSurveyState"} {
		if _, ok := result[key]; ok {
			t.Errorf("expected key %q to be dropped", key)
		}
	}
}

func TestTransformSettings_PreservesValues(t *testing.T) {
	input := `{"alwaysThinkingEnabled": true, "permissions": {"allow": ["Bash", "Read"]}}`

	out, err := TransformSettings([]byte(input))
	if err != nil {
		t.Fatalf("TransformSettings error: %v", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Check alwaysThinkingEnabled value
	var thinking bool
	if err := json.Unmarshal(result["alwaysThinkingEnabled"], &thinking); err != nil {
		t.Fatalf("unmarshal thinking: %v", err)
	}
	if !thinking {
		t.Error("alwaysThinkingEnabled should be true")
	}

	// Check permissions preserved
	var perms map[string][]string
	if err := json.Unmarshal(result["permissions"], &perms); err != nil {
		t.Fatalf("unmarshal permissions: %v", err)
	}
	if len(perms["allow"]) != 2 {
		t.Errorf("permissions.allow length = %d, want 2", len(perms["allow"]))
	}
}

func TestTransformSettings_EmptyObject(t *testing.T) {
	out, err := TransformSettings([]byte(`{}`))
	if err != nil {
		t.Fatalf("TransformSettings error: %v", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty object, got %d keys", len(result))
	}
}

func TestTransformSettings_OnlyDroppedFields(t *testing.T) {
	input := `{"hooks": {"pre": "test"}, "statusLine": "x", "feedbackSurveyState": {}}`

	out, err := TransformSettings([]byte(input))
	if err != nil {
		t.Fatalf("TransformSettings error: %v", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty object, got %d keys", len(result))
	}
}

func TestTransformSettings_InvalidJSON(t *testing.T) {
	_, err := TransformSettings([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing settings.json") {
		t.Errorf("error = %q, want to contain 'parsing settings.json'", err.Error())
	}
}

func TestTransformSettings_UnknownFieldsDropped(t *testing.T) {
	input := `{"alwaysThinkingEnabled": true, "someFutureField": 42, "anotherNew": "value"}`

	out, err := TransformSettings([]byte(input))
	if err != nil {
		t.Fatalf("TransformSettings error: %v", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 key, got %d", len(result))
	}
	if _, ok := result["alwaysThinkingEnabled"]; !ok {
		t.Error("expected alwaysThinkingEnabled to be present")
	}
}

func TestClaudeSessionName_ForStateSync(t *testing.T) {
	tests := []struct {
		project string
		want    string
	}{
		{"myapp", "fusebox-claude-myapp"},
		{"web-frontend", "fusebox-claude-web-frontend"},
		{"a", "fusebox-claude-a"},
	}
	for _, tt := range tests {
		t.Run(tt.project, func(t *testing.T) {
			got := ClaudeSessionName(tt.project)
			if got != tt.want {
				t.Errorf("ClaudeSessionName(%q) = %q, want %q", tt.project, got, tt.want)
			}
		})
	}
}

func TestCreateClaudeStateSync_CommandArgs(t *testing.T) {
	runner := &mockRunner{results: []mockResult{{}}}
	mgr := NewMutagenManagerWithRunner(runner)

	// We need to override the home dir lookup for deterministic testing.
	// Instead, test the underlying Create call by verifying the runner received correct args.
	// CreateClaudeStateSync calls manager.Create which calls runner.Run.

	// Use a known home dir by calling Create directly with the paths we'd expect.
	localEncoded := EncodePath("/Users/gk/work/project")
	remoteEncoded := EncodePath("/workspace/project")

	localSyncPath := "/Users/gk/.claude/projects/" + localEncoded
	remoteSyncPath := "/home/deploy/.claude/projects/" + remoteEncoded
	sessionName := ClaudeSessionName("project")

	err := mgr.Create(sessionName, localSyncPath, "deploy", "10.0.0.1", remoteSyncPath, nil)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	call := runner.calls[0]
	got := strings.Join(call.Args, " ")
	want := "sync create /Users/gk/.claude/projects/-Users-gk-work-project deploy@10.0.0.1:/home/deploy/.claude/projects/-workspace-project --name fusebox-claude-project --ignore .git --ignore fusebox.yaml"
	if got != want {
		t.Errorf("args =\n  %s\nwant:\n  %s", got, want)
	}
}

// mockSSHClient records CopyFile and RunCommand calls for testing CopyGlobalState.
type mockSSHClient struct {
	copiedFiles []copyCall
	commands    []string
	copyErr     error
	runResult   struct {
		stdout   string
		stderr   string
		exitCode int
		err      error
	}
}

type copyCall struct {
	Local  string
	Remote string
}

func (m *mockSSHClient) CopyFile(localPath, remotePath string) error {
	m.copiedFiles = append(m.copiedFiles, copyCall{Local: localPath, Remote: remotePath})
	return m.copyErr
}

func (m *mockSSHClient) RunCommand(cmd string) (string, string, int, error) {
	m.commands = append(m.commands, cmd)
	return m.runResult.stdout, m.runResult.stderr, m.runResult.exitCode, m.runResult.err
}

func TestCopyGlobalState_CreatesDirectories(t *testing.T) {
	sshClient := &mockSSHClient{}

	// CopyGlobalState reads from the real home dir. Some files may not exist,
	// which is OK — we just verify it calls mkdir and doesn't error on missing files.
	err := CopyGlobalState(sshClient, "deploy")
	if err != nil {
		t.Fatalf("CopyGlobalState error: %v", err)
	}

	if len(sshClient.commands) < 1 {
		t.Fatal("expected at least 1 RunCommand call for mkdir")
	}

	mkdirCmd := sshClient.commands[0]
	if !strings.Contains(mkdirCmd, "mkdir -p") {
		t.Errorf("first command = %q, want mkdir -p", mkdirCmd)
	}
	if !strings.Contains(mkdirCmd, "/home/deploy/.claude") {
		t.Errorf("mkdir target missing /home/deploy/.claude: %q", mkdirCmd)
	}
}

func TestCopyGlobalState_MkdirFailure(t *testing.T) {
	sshClient := &mockSSHClient{}
	sshClient.runResult.exitCode = 1
	sshClient.runResult.stderr = "permission denied"

	err := CopyGlobalState(sshClient, "deploy")
	if err == nil {
		t.Fatal("expected error on mkdir failure")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want to contain 'permission denied'", err.Error())
	}
}

func TestCopyGlobalState_MkdirRunError(t *testing.T) {
	sshClient := &mockSSHClient{}
	sshClient.runResult.err = fmt.Errorf("connection reset")

	err := CopyGlobalState(sshClient, "deploy")
	if err == nil {
		t.Fatal("expected error on RunCommand failure")
	}
	if !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("error = %q, want to contain 'connection reset'", err.Error())
	}
}

func TestCopyDir_NonexistentDir(t *testing.T) {
	sshClient := &mockSSHClient{}

	err := copyDir(sshClient, "/nonexistent/path/that/does/not/exist", "/remote/dir")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}

	if len(sshClient.copiedFiles) != 0 {
		t.Errorf("expected no files copied, got %d", len(sshClient.copiedFiles))
	}
}

func TestCopyDir_WithFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/file1.md", []byte("content1"), 0644)
	os.WriteFile(dir+"/file2.json", []byte("content2"), 0644)
	os.Mkdir(dir+"/subdir", 0755) // should be skipped

	sshClient := &mockSSHClient{}

	err := copyDir(sshClient, dir, "/remote/target")
	if err != nil {
		t.Fatalf("copyDir error: %v", err)
	}

	if len(sshClient.copiedFiles) != 2 {
		t.Fatalf("copied files = %d, want 2", len(sshClient.copiedFiles))
	}
}

func TestCopyDir_CopyError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/file.md", []byte("content"), 0644)

	sshClient := &mockSSHClient{copyErr: fmt.Errorf("scp failed")}

	err := copyDir(sshClient, dir, "/remote/target")
	if err == nil {
		t.Fatal("expected error on copy failure")
	}
	if !strings.Contains(err.Error(), "scp failed") {
		t.Errorf("error = %q, want to contain 'scp failed'", err.Error())
	}
}
