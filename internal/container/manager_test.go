package container

import (
	"fmt"
	"strings"
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

// mockRunner records commands for testing Manager methods.
type mockRunner struct {
	commands []string
	// responses maps command prefix to (stdout, stderr, exitCode, err)
	responses map[string]mockResponse
	// defaultResponse used when no prefix match
	defaultResponse mockResponse
}

type mockResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		responses: make(map[string]mockResponse),
		defaultResponse: mockResponse{
			stdout: "", stderr: "", exitCode: 0, err: nil,
		},
	}
}

func (m *mockRunner) RunCommand(cmd string) (string, string, int, error) {
	m.commands = append(m.commands, cmd)
	for prefix, resp := range m.responses {
		if strings.HasPrefix(cmd, prefix) {
			return resp.stdout, resp.stderr, resp.exitCode, resp.err
		}
	}
	return m.defaultResponse.stdout, m.defaultResponse.stderr, m.defaultResponse.exitCode, m.defaultResponse.err
}

func TestCreate_NoTokenInDockerRun(t *testing.T) {
	mock := newMockRunner()
	// EnsureImage: image already exists
	mock.responses["docker image inspect"] = mockResponse{exitCode: 0}
	mgr := newManagerWithRunner(mock)

	token := "super-secret-oauth-token"
	err := mgr.Create("myproject", token, 60001)
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	// Verify docker run command does NOT contain the token as an env var
	for _, cmd := range mock.commands {
		if strings.HasPrefix(cmd, "docker run") {
			if strings.Contains(cmd, "-e CLAUDE_CODE_OAUTH_TOKEN") {
				t.Error("docker run still contains -e CLAUDE_CODE_OAUTH_TOKEN; token visible in process listing")
			}
			if strings.Contains(cmd, token) {
				t.Error("docker run command contains the raw token")
			}
		}
	}
}

func TestCreate_InjectsTokenViaDockerExec(t *testing.T) {
	mock := newMockRunner()
	mock.responses["docker image inspect"] = mockResponse{exitCode: 0}
	mgr := newManagerWithRunner(mock)

	token := "test-token-value"
	err := mgr.Create("myproject", token, 60001)
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}

	// Verify a docker exec command was issued to inject the token
	foundExec := false
	for _, cmd := range mock.commands {
		if strings.HasPrefix(cmd, "docker exec") {
			foundExec = true
			if !strings.Contains(cmd, "/root/.fusebox/token") {
				t.Error("docker exec does not write to /root/.fusebox/token")
			}
			if !strings.Contains(cmd, "chmod 600") {
				t.Error("docker exec does not chmod 600 the token file")
			}
		}
	}
	if !foundExec {
		t.Error("no docker exec command issued to inject the token")
	}
}

func TestCreate_CleansUpOnTokenInjectionFailure(t *testing.T) {
	mock := newMockRunner()
	mock.responses["docker image inspect"] = mockResponse{exitCode: 0}
	mock.responses["docker exec"] = mockResponse{exitCode: 1, stderr: "exec failed"}
	mgr := newManagerWithRunner(mock)

	err := mgr.Create("myproject", "tok", 60001)
	if err == nil {
		t.Fatal("Create() should have returned an error when token injection fails")
	}
	if !strings.Contains(err.Error(), "injecting token") {
		t.Errorf("error should mention injecting token, got: %v", err)
	}

	// Verify cleanup: docker rm -f was called
	foundCleanup := false
	for _, cmd := range mock.commands {
		if strings.HasPrefix(cmd, "docker rm -f") {
			foundCleanup = true
		}
	}
	if !foundCleanup {
		t.Error("container not cleaned up after token injection failure")
	}
}

func TestInjectToken_Success(t *testing.T) {
	mock := newMockRunner()
	mgr := newManagerWithRunner(mock)

	err := mgr.InjectToken("fusebox-myproject", "my-token")
	if err != nil {
		t.Fatalf("InjectToken() returned error: %v", err)
	}

	if len(mock.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.commands))
	}
	cmd := mock.commands[0]
	if !strings.HasPrefix(cmd, "docker exec fusebox-myproject") {
		t.Errorf("expected docker exec on the container, got: %s", cmd)
	}
}

func TestInjectToken_Failure(t *testing.T) {
	mock := newMockRunner()
	mock.responses["docker exec"] = mockResponse{exitCode: 1, stderr: "container not running"}
	mgr := newManagerWithRunner(mock)

	err := mgr.InjectToken("fusebox-myproject", "my-token")
	if err == nil {
		t.Fatal("InjectToken() should return error on failure")
	}
	if !strings.Contains(err.Error(), "token injection failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInjectToken_SSHError(t *testing.T) {
	mock := newMockRunner()
	mock.defaultResponse = mockResponse{err: fmt.Errorf("ssh connection closed")}
	mgr := newManagerWithRunner(mock)

	err := mgr.InjectToken("fusebox-myproject", "my-token")
	if err == nil {
		t.Fatal("InjectToken() should return error on SSH failure")
	}
	if !strings.Contains(err.Error(), "exec into container") {
		t.Errorf("unexpected error: %v", err)
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
