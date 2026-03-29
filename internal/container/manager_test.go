package container

import (
	"testing"
)

func TestContainerName(t *testing.T) {
	tests := []struct {
		project string
		want    string
	}{
		{"myapp", "fusebox-myapp"},
		{"web-frontend", "fusebox-web-frontend"},
		{"a", "fusebox-a"},
	}

	for _, tt := range tests {
		got := ContainerName(tt.project)
		if got != tt.want {
			t.Errorf("ContainerName(%q) = %q, want %q", tt.project, got, tt.want)
		}
	}
}

func TestNextAvailablePort_Empty(t *testing.T) {
	ports := make(PortMap)
	got := nextAvailablePort(ports)
	if got != basePort {
		t.Errorf("nextAvailablePort(empty) = %d, want %d", got, basePort)
	}
}

func TestNextAvailablePort_Sequential(t *testing.T) {
	ports := PortMap{
		"project-a": 60001,
		"project-b": 60002,
	}
	got := nextAvailablePort(ports)
	if got != 60003 {
		t.Errorf("nextAvailablePort = %d, want 60003", got)
	}
}

func TestNextAvailablePort_ReusesFreedPort(t *testing.T) {
	ports := PortMap{
		"project-a": 60001,
		"project-c": 60003,
	}
	got := nextAvailablePort(ports)
	if got != 60002 {
		t.Errorf("nextAvailablePort = %d, want 60002 (reused gap)", got)
	}
}

func TestNextAvailablePort_AllContiguous(t *testing.T) {
	ports := PortMap{
		"a": 60001,
		"b": 60002,
		"c": 60003,
		"d": 60004,
	}
	got := nextAvailablePort(ports)
	if got != 60005 {
		t.Errorf("nextAvailablePort = %d, want 60005", got)
	}
}

func TestParseInspectOutput_Running(t *testing.T) {
	output := "running|0|2026-03-28T10:00:00Z"
	got := ParseInspectOutput(output)
	if got.State != StateRunning {
		t.Errorf("state = %q, want %q", got.State, StateRunning)
	}
	if got.Uptime != "2026-03-28T10:00:00Z" {
		t.Errorf("uptime = %q, want %q", got.Uptime, "2026-03-28T10:00:00Z")
	}
}

func TestParseInspectOutput_Stopped(t *testing.T) {
	output := "exited|0|2026-03-28T09:00:00Z"
	got := ParseInspectOutput(output)
	if got.State != StateStopped {
		t.Errorf("state = %q, want %q", got.State, StateStopped)
	}
	if got.ExitCode != 0 {
		t.Errorf("exitCode = %d, want 0", got.ExitCode)
	}
}

func TestParseInspectOutput_Crashed(t *testing.T) {
	output := "exited|137|2026-03-28T09:00:00Z"
	got := ParseInspectOutput(output)
	if got.State != StateCrashed {
		t.Errorf("state = %q, want %q", got.State, StateCrashed)
	}
	if got.ExitCode != 137 {
		t.Errorf("exitCode = %d, want 137", got.ExitCode)
	}
}

func TestParseInspectOutput_BadFormat(t *testing.T) {
	got := ParseInspectOutput("garbage")
	if got.State != StateNotFound {
		t.Errorf("state = %q, want %q", got.State, StateNotFound)
	}
}

func TestParseInspectOutput_UnknownStatus(t *testing.T) {
	output := "paused|0|2026-03-28T10:00:00Z"
	got := ParseInspectOutput(output)
	if got.State != StateStopped {
		t.Errorf("state = %q, want %q for unknown docker status", got.State, StateStopped)
	}
}

func TestGenerateDockerfile(t *testing.T) {
	df := GenerateDockerfile()
	if df == "" {
		t.Fatal("GenerateDockerfile() returned empty string")
	}
	if !contains(df, "nestybox/ubuntu-jammy-systemd-docker") {
		t.Error("Dockerfile missing base image")
	}
	if !contains(df, "claude-code") {
		t.Error("Dockerfile missing claude-code install")
	}
	if !contains(df, "mosh") {
		t.Error("Dockerfile missing mosh install")
	}
	if !contains(df, "nodejs") {
		t.Error("Dockerfile missing nodejs install")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"it's", "'it'\"'\"'s'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
