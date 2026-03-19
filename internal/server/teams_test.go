package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- updateTeamsSettings tests ---

func TestUpdateTeamsSettings_EnableEmpty(t *testing.T) {
	out, msg, err := updateTeamsSettings(nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "teams enabled" {
		t.Errorf("msg = %q, want %q", msg, "teams enabled")
	}

	var settings map[string]any
	if err := json.Unmarshal(out, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	env := settings["env"].(map[string]any)
	if env["CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"] != "1" {
		t.Errorf("expected env var set to 1, got %v", env["CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"])
	}
}

func TestUpdateTeamsSettings_EnableIdempotent(t *testing.T) {
	input := []byte(`{"env":{"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS":"1"}}`)
	out, msg, err := updateTeamsSettings(input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "no change" {
		t.Errorf("msg = %q, want %q", msg, "no change")
	}
	if string(out) != string(input) {
		t.Error("expected original data returned unchanged")
	}
}

func TestUpdateTeamsSettings_Disable(t *testing.T) {
	input := []byte(`{"env":{"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS":"1","OTHER":"val"}}`)
	out, msg, err := updateTeamsSettings(input, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "teams disabled" {
		t.Errorf("msg = %q, want %q", msg, "teams disabled")
	}

	var settings map[string]any
	json.Unmarshal(out, &settings)
	env := settings["env"].(map[string]any)
	if _, exists := env["CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"]; exists {
		t.Error("expected env var removed")
	}
	if env["OTHER"] != "val" {
		t.Error("expected other env vars preserved")
	}
}

func TestUpdateTeamsSettings_DisableRemovesEmptyEnv(t *testing.T) {
	input := []byte(`{"env":{"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS":"1"},"other":true}`)
	out, msg, err := updateTeamsSettings(input, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "teams disabled" {
		t.Errorf("msg = %q, want %q", msg, "teams disabled")
	}

	var settings map[string]any
	json.Unmarshal(out, &settings)
	if _, exists := settings["env"]; exists {
		t.Error("expected empty env block removed")
	}
	if settings["other"] != true {
		t.Error("expected other settings preserved")
	}
}

func TestUpdateTeamsSettings_DisableIdempotent(t *testing.T) {
	input := []byte(`{"other":true}`)
	out, msg, err := updateTeamsSettings(input, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "no change" {
		t.Errorf("msg = %q, want %q", msg, "no change")
	}
	if string(out) != string(input) {
		t.Error("expected original data returned unchanged")
	}
}

func TestUpdateTeamsSettings_InvalidJSON(t *testing.T) {
	_, _, err := updateTeamsSettings([]byte("not json"), true)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestUpdateTeamsSettings_PreservesExistingSettings(t *testing.T) {
	input := []byte(`{"hooks":{"PostToolUse":[]},"env":{"PATH":"/usr/bin"}}`)
	out, msg, err := updateTeamsSettings(input, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "teams enabled" {
		t.Errorf("msg = %q, want %q", msg, "teams enabled")
	}

	var settings map[string]any
	json.Unmarshal(out, &settings)
	if settings["hooks"] == nil {
		t.Error("expected hooks preserved")
	}
	env := settings["env"].(map[string]any)
	if env["PATH"] != "/usr/bin" {
		t.Error("expected PATH preserved")
	}
	if env["CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"] != "1" {
		t.Error("expected teams env var set")
	}
}

// --- parsePaneLine tests ---

func TestParsePaneLine(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    paneInfo
		wantErr bool
	}{
		{
			name:  "active pane",
			input: "0|lead|claude|1",
			want:  paneInfo{Index: 0, Title: "lead", Command: "claude", Active: true},
		},
		{
			name:  "inactive pane",
			input: "1|researcher|claude|0",
			want:  paneInfo{Index: 1, Title: "researcher", Command: "claude", Active: false},
		},
		{
			name:  "pane with empty title",
			input: "2||bash|0",
			want:  paneInfo{Index: 2, Title: "", Command: "bash", Active: false},
		},
		{
			name:    "too few fields",
			input:   "0|title",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePaneLine(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// --- loadTeams tests ---

func TestLoadTeams_MissingDirs(t *testing.T) {
	teams := loadTeams("/nonexistent/path")
	if len(teams) != 0 {
		t.Errorf("expected empty, got %d teams", len(teams))
	}
}

func TestLoadTeams_EmptyTeamsDir(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude", "teams"), 0755)

	teams := loadTeams(home)
	if len(teams) != 0 {
		t.Errorf("expected empty, got %d teams", len(teams))
	}
}

func TestLoadTeams_TeamWithConfig(t *testing.T) {
	home := t.TempDir()
	teamDir := filepath.Join(home, ".claude", "teams", "auth-review")
	os.MkdirAll(teamDir, 0755)

	config := `{"members":[{"name":"lead"},{"name":"researcher","agent_type":"research"}]}`
	os.WriteFile(filepath.Join(teamDir, "config.json"), []byte(config), 0644)

	teams := loadTeams(home)
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	if teams[0].Name != "auth-review" {
		t.Errorf("name = %q, want %q", teams[0].Name, "auth-review")
	}
	if len(teams[0].Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(teams[0].Members))
	}
}

func TestLoadTeams_TeamWithTasks(t *testing.T) {
	home := t.TempDir()
	teamDir := filepath.Join(home, ".claude", "teams", "my-team")
	os.MkdirAll(teamDir, 0755)
	os.WriteFile(filepath.Join(teamDir, "config.json"), []byte(`{"members":[]}`), 0644)

	taskDir := filepath.Join(home, ".claude", "tasks", "my-team")
	os.MkdirAll(taskDir, 0755)

	os.WriteFile(filepath.Join(taskDir, "task1.json"),
		[]byte(`{"id":"task1","title":"Review auth","state":"completed"}`), 0644)
	os.WriteFile(filepath.Join(taskDir, "task2.json"),
		[]byte(`{"id":"task2","title":"Write tests","state":"in_progress","assigned_to":"researcher"}`), 0644)
	os.WriteFile(filepath.Join(taskDir, "task3.json"),
		[]byte(`{"title":"Update docs","state":"pending"}`), 0644)

	teams := loadTeams(home)
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	ts := teams[0]
	if ts.Total != 3 {
		t.Errorf("total = %d, want 3", ts.Total)
	}
	if ts.Completed != 1 {
		t.Errorf("completed = %d, want 1", ts.Completed)
	}
	if ts.InProgress != 1 {
		t.Errorf("in_progress = %d, want 1", ts.InProgress)
	}
	if ts.Pending != 1 {
		t.Errorf("pending = %d, want 1", ts.Pending)
	}
	// Task without ID should get filename-based ID
	for _, task := range ts.Tasks {
		if task.Title == "Update docs" && task.ID != "task3" {
			t.Errorf("expected task3 ID from filename, got %q", task.ID)
		}
	}
}

func TestLoadTeams_MalformedFilesSkipped(t *testing.T) {
	home := t.TempDir()
	teamDir := filepath.Join(home, ".claude", "teams", "bad-team")
	os.MkdirAll(teamDir, 0755)
	os.WriteFile(filepath.Join(teamDir, "config.json"), []byte("not json"), 0644)

	taskDir := filepath.Join(home, ".claude", "tasks", "bad-team")
	os.MkdirAll(taskDir, 0755)
	os.WriteFile(filepath.Join(taskDir, "bad.json"), []byte("{broken"), 0644)

	teams := loadTeams(home)
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	// Should have team entry but no members/tasks (malformed files skipped)
	if len(teams[0].Members) != 0 {
		t.Errorf("expected 0 members, got %d", len(teams[0].Members))
	}
	if len(teams[0].Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(teams[0].Tasks))
	}
}
