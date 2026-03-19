package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSessionLine(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    sessionInfo
		wantErr bool
	}{
		{
			name:  "normal session",
			input: "mysession|/home/user/projects|1710000000|1710000100",
			want:  sessionInfo{"mysession", "/home/user/projects", 1710000000, 1710000100},
		},
		{
			name:  "zero timestamps",
			input: "test|/tmp|0|0",
			want:  sessionInfo{"test", "/tmp", 0, 0},
		},
		{
			name:  "path with spaces",
			input: "dev|/home/user/my projects|1710000000|1710000100",
			want:  sessionInfo{"dev", "/home/user/my projects", 1710000000, 1710000100},
		},
		{
			name:    "too few fields",
			input:   "only|two",
			wantErr: true,
		},
		{
			name:    "bad created timestamp",
			input:   "test|/tmp|notanumber|0",
			wantErr: true,
		},
		{
			name:    "bad activity timestamp",
			input:   "test|/tmp|0|notanumber",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSessionLine(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractDetail(t *testing.T) {
	tests := []struct {
		name string
		tool string
		ti   toolInput
		want string
	}{
		{"bash short command", "Bash", toolInput{Command: "ls -la"}, "ls -la"},
		{"bash truncated at 30", "Bash", toolInput{Command: "ls -la /some/very/long/path/that/exceeds/thirty"}, "ls -la /some/very/long/path/th"},
		{"bash multiline takes first line", "Bash", toolInput{Command: "echo hello\necho world"}, "echo hello"},
		{"bash empty command", "Bash", toolInput{}, ""},
		{"edit extracts basename", "Edit", toolInput{FilePath: "/home/user/projects/app.go"}, "app.go"},
		{"read extracts basename", "Read", toolInput{FilePath: "/tmp/test.txt"}, "test.txt"},
		{"write extracts basename", "Write", toolInput{FilePath: "/home/user/main.go"}, "main.go"},
		{"edit empty path", "Edit", toolInput{}, ""},
		{"grep short pattern", "Grep", toolInput{Pattern: "**/*.go"}, "**/*.go"},
		{"grep truncated at 20", "Grep", toolInput{Pattern: "func TestSomethingVeryLong"}, "func TestSomethingVe"},
		{"glob pattern", "Glob", toolInput{Pattern: "*.ts"}, "*.ts"},
		{"agent always subagent", "Agent", toolInput{}, "subagent"},
		{"unknown tool empty", "SomethingElse", toolInput{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := hookInput{ToolName: tt.tool, ToolInput: tt.ti}
			got := extractDetail(input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRoots(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"multiple roots", "/home/user/projects\n/home/user/work\n", []string{"/home/user/projects", "/home/user/work"}},
		{"empty string", "", nil},
		{"only newlines", "\n\n", nil},
		{"single root", "/single\n", []string{"/single"}},
		{"whitespace trimmed", "  /home/user/projects  \n  /tmp  \n", []string{"/home/user/projects", "/tmp"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRoots(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("got %d items, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- hooks settings tests ---

func TestUpdateHooksSettings_EmptySettings(t *testing.T) {
	out, msg, err := updateHooksSettings(nil, "/usr/bin/fusebox-server hook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "hook installed" {
		t.Errorf("msg = %q, want %q", msg, "hook installed")
	}

	var settings map[string]any
	if err := json.Unmarshal(out, &settings); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	// Verify structure
	hooks := settings["hooks"].(map[string]any)
	postToolUse := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("expected 1 PostToolUse entry, got %d", len(postToolUse))
	}
	entry := postToolUse[0].(map[string]any)
	if entry["matcher"] != "" {
		t.Errorf("matcher = %q, want empty", entry["matcher"])
	}
	hooksList := entry["hooks"].([]any)
	hook := hooksList[0].(map[string]any)
	if hook["command"] != "/usr/bin/fusebox-server hook" {
		t.Errorf("command = %q, want %q", hook["command"], "/usr/bin/fusebox-server hook")
	}
}

func TestUpdateHooksSettings_AlreadyInstalled(t *testing.T) {
	input := []byte(`{
  "hooks": {
    "PostToolUse": [
      {"matcher": "", "hooks": [{"type": "command", "command": "/usr/bin/fusebox-server hook"}]}
    ]
  }
}`)
	out, msg, err := updateHooksSettings(input, "/usr/bin/fusebox-server hook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "hook already installed" {
		t.Errorf("msg = %q, want %q", msg, "hook already installed")
	}
	// Should return original data unchanged
	if string(out) != string(input) {
		t.Error("expected original data returned unchanged")
	}
}

func TestUpdateHooksSettings_RemovesOldWorkHook(t *testing.T) {
	input := []byte(`{
  "hooks": {
    "PostToolUse": [
      {"matcher": "", "hooks": [{"type": "command", "command": "/home/user/bin/fusebox-hook"}]}
    ]
  }
}`)
	out, msg, err := updateHooksSettings(input, "/home/user/bin/fusebox-server hook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "hook installed" {
		t.Errorf("msg = %q, want %q", msg, "hook installed")
	}

	var settings map[string]any
	json.Unmarshal(out, &settings)
	hooks := settings["hooks"].(map[string]any)
	postToolUse := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("expected 1 entry (old removed, new added), got %d", len(postToolUse))
	}
	entry := postToolUse[0].(map[string]any)
	hooksList := entry["hooks"].([]any)
	hook := hooksList[0].(map[string]any)
	if hook["command"] != "/home/user/bin/fusebox-server hook" {
		t.Errorf("command = %q, want %q", hook["command"], "/home/user/bin/fusebox-server hook")
	}
}

func TestUpdateHooksSettings_RemovesOldFormat(t *testing.T) {
	// Old format: entries without "hooks" key (just had "type"/"command" at top level)
	input := []byte(`{
  "hooks": {
    "PostToolUse": [
      {"type": "command", "command": "/home/user/bin/fusebox-hook"}
    ]
  }
}`)
	out, msg, err := updateHooksSettings(input, "/home/user/bin/fusebox-server hook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "hook installed" {
		t.Errorf("msg = %q, want %q", msg, "hook installed")
	}

	var settings map[string]any
	json.Unmarshal(out, &settings)
	hooks := settings["hooks"].(map[string]any)
	postToolUse := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 1 {
		t.Fatalf("expected 1 entry (old removed, new added), got %d", len(postToolUse))
	}
}

func TestUpdateHooksSettings_PreservesOtherHooks(t *testing.T) {
	input := []byte(`{
  "hooks": {
    "PostToolUse": [
      {"matcher": "*.py", "hooks": [{"type": "command", "command": "/usr/bin/other-hook"}]}
    ]
  },
  "other_setting": true
}`)
	out, msg, err := updateHooksSettings(input, "/home/user/bin/fusebox-server hook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "hook installed" {
		t.Errorf("msg = %q, want %q", msg, "hook installed")
	}

	var settings map[string]any
	json.Unmarshal(out, &settings)

	// Other settings preserved
	if settings["other_setting"] != true {
		t.Error("other_setting should be preserved")
	}

	hooks := settings["hooks"].(map[string]any)
	postToolUse := hooks["PostToolUse"].([]any)
	if len(postToolUse) != 2 {
		t.Fatalf("expected 2 entries (other + new), got %d", len(postToolUse))
	}

	// First entry should be the other hook
	entry0 := postToolUse[0].(map[string]any)
	if entry0["matcher"] != "*.py" {
		t.Errorf("first entry matcher = %q, want %q", entry0["matcher"], "*.py")
	}

	// Second entry should be our new hook
	entry1 := postToolUse[1].(map[string]any)
	hooksList := entry1["hooks"].([]any)
	hook := hooksList[0].(map[string]any)
	if hook["command"] != "/home/user/bin/fusebox-server hook" {
		t.Errorf("new hook command = %q", hook["command"])
	}
}

func TestUpdateHooksSettings_InvalidJSON(t *testing.T) {
	_, _, err := updateHooksSettings([]byte("not json"), "/usr/bin/fusebox-server hook")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- activity tests ---

func TestReadActivityDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := readActivityDir(dir, 60)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestReadActivityDir_NonexistentDir(t *testing.T) {
	result := readActivityDir("/nonexistent/dir/that/does/not/exist", 60)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestReadActivityDir_RecentActivity(t *testing.T) {
	dir := t.TempDir()

	// Write a recent activity file
	now := time.Now().Unix()
	data := fmt.Sprintf(`{"tool":"Edit","detail":"app.go","ts":%d}`, now)
	os.WriteFile(filepath.Join(dir, "mysession.json"), []byte(data), 0644)

	result := readActivityDir(dir, 60)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if _, ok := result["mysession"]; !ok {
		t.Error("expected 'mysession' key in result")
	}

	// Verify the JSON is preserved verbatim
	var activity map[string]any
	json.Unmarshal(result["mysession"], &activity)
	if activity["tool"] != "Edit" {
		t.Errorf("tool = %v, want Edit", activity["tool"])
	}
}

func TestReadActivityDir_StaleActivityFiltered(t *testing.T) {
	dir := t.TempDir()

	// Write an old activity file (120 seconds ago)
	old := time.Now().Unix() - 120
	data := fmt.Sprintf(`{"tool":"Bash","detail":"ls","ts":%d}`, old)
	os.WriteFile(filepath.Join(dir, "oldsession.json"), []byte(data), 0644)

	result := readActivityDir(dir, 60)
	if len(result) != 0 {
		t.Errorf("expected stale entry to be filtered, got %d entries", len(result))
	}
}

func TestReadActivityDir_MixedRecentAndStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Unix()

	// Recent
	os.WriteFile(filepath.Join(dir, "active.json"),
		[]byte(fmt.Sprintf(`{"tool":"Edit","detail":"x.go","ts":%d}`, now)), 0644)
	// Stale
	os.WriteFile(filepath.Join(dir, "stale.json"),
		[]byte(fmt.Sprintf(`{"tool":"Bash","detail":"ls","ts":%d}`, now-120)), 0644)
	// Not JSON
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not json"), 0644)
	// Invalid JSON
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{broken"), 0644)
	// Subdirectory (should be skipped)
	os.MkdirAll(filepath.Join(dir, "subdir.json"), 0755)

	result := readActivityDir(dir, 60)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
	if _, ok := result["active"]; !ok {
		t.Error("expected 'active' key in result")
	}
}
