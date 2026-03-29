package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// --- Mock daemon client ---

type mockDaemon struct {
	actions    []ActionDescriptor
	execResult *ExecResult
	execErr    error
	actionsErr error
}

func (m *mockDaemon) RequestActions() ([]ActionDescriptor, error) {
	if m.actionsErr != nil {
		return nil, m.actionsErr
	}
	return m.actions, nil
}

func (m *mockDaemon) Exec(action string, params map[string]string) (*ExecResult, error) {
	if m.execErr != nil {
		return nil, m.execErr
	}
	return m.execResult, nil
}

func (m *mockDaemon) Close() error { return nil }

// --- Helpers ---

func sendRequest(t *testing.T, method string, id int, params interface{}) string {
	t.Helper()
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		p, _ := json.Marshal(params)
		req["params"] = json.RawMessage(p)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshaling request: %v", err)
	}
	return string(data)
}

func parseResponse(t *testing.T, line string) jsonrpcResponse {
	t.Helper()
	var resp jsonrpcResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("parsing response %q: %v", line, err)
	}
	return resp
}

func runServer(t *testing.T, daemon DaemonClient, input string) []string {
	t.Helper()
	in := strings.NewReader(input)
	var out bytes.Buffer
	srv := NewServer(daemon, in, &out)
	if err := srv.Serve(); err != nil {
		t.Fatalf("serve error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// --- Tests ---

func TestInitialize(t *testing.T) {
	daemon := &mockDaemon{}
	input := sendRequest(t, "initialize", 1, nil)
	lines := runServer(t, daemon, input)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response, got %d", len(lines))
	}

	resp := parseResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, _ := json.Marshal(resp.Result)
	var r map[string]interface{}
	json.Unmarshal(result, &r)

	info := r["serverInfo"].(map[string]interface{})
	if info["name"] != "fusebox" {
		t.Fatalf("expected server name 'fusebox', got %v", info["name"])
	}
	if r["protocolVersion"] != "2024-11-05" {
		t.Fatalf("unexpected protocol version: %v", r["protocolVersion"])
	}
}

func TestToolsList_Empty(t *testing.T) {
	daemon := &mockDaemon{actions: []ActionDescriptor{}}
	input := sendRequest(t, "tools/list", 1, nil)
	lines := runServer(t, daemon, input)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response, got %d", len(lines))
	}

	resp := parseResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, _ := json.Marshal(resp.Result)
	var r map[string]interface{}
	json.Unmarshal(result, &r)

	tools := r["tools"].([]interface{})
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(tools))
	}
}

func TestToolsList_WithActions(t *testing.T) {
	daemon := &mockDaemon{
		actions: []ActionDescriptor{
			{
				Name:        "build",
				Description: "Build the project",
				Params: map[string]ParamDescriptor{
					"target": {Type: "enum", Values: []string{"debug", "release"}},
				},
			},
			{
				Name:        "test",
				Description: "Run tests",
				Params:      nil,
			},
		},
	}
	input := sendRequest(t, "tools/list", 1, nil)
	lines := runServer(t, daemon, input)

	resp := parseResponse(t, lines[0])
	result, _ := json.Marshal(resp.Result)
	var r map[string]interface{}
	json.Unmarshal(result, &r)

	tools := r["tools"].([]interface{})
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Verify tool names are prefixed.
	names := make(map[string]bool)
	for _, tool := range tools {
		tm := tool.(map[string]interface{})
		names[tm["name"].(string)] = true
	}
	if !names["fusebox_build"] || !names["fusebox_test"] {
		t.Fatalf("expected fusebox_build and fusebox_test, got %v", names)
	}
}

func TestToolsList_DaemonUnreachable(t *testing.T) {
	daemon := &mockDaemon{
		actionsErr: fmt.Errorf("local machine unreachable: connection refused"),
	}
	input := sendRequest(t, "tools/list", 1, nil)
	lines := runServer(t, daemon, input)

	resp := parseResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("expected graceful empty tools, got error: %v", resp.Error)
	}

	result, _ := json.Marshal(resp.Result)
	var r map[string]interface{}
	json.Unmarshal(result, &r)

	tools := r["tools"].([]interface{})
	if len(tools) != 0 {
		t.Fatalf("expected 0 tools when unreachable, got %d", len(tools))
	}
}

func TestToolsCall_Success(t *testing.T) {
	daemon := &mockDaemon{
		execResult: &ExecResult{
			Output:     "build complete\n",
			ExitCode:   0,
			DurationMs: 1234,
		},
	}
	input := sendRequest(t, "tools/call", 1, map[string]interface{}{
		"name":      "fusebox_build",
		"arguments": map[string]interface{}{"target": "release"},
	})
	lines := runServer(t, daemon, input)

	resp := parseResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, _ := json.Marshal(resp.Result)
	var r map[string]interface{}
	json.Unmarshal(result, &r)

	content := r["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if !strings.Contains(text, `"exit_code":0`) {
		t.Fatalf("expected exit_code 0 in result, got: %s", text)
	}
	if !strings.Contains(text, `"duration_ms":1234`) {
		t.Fatalf("expected duration_ms in result, got: %s", text)
	}

	isError := r["isError"].(bool)
	if isError {
		t.Fatal("expected isError=false for exit code 0")
	}
}

func TestToolsCall_NonZeroExit(t *testing.T) {
	daemon := &mockDaemon{
		execResult: &ExecResult{
			Output:     "error: test failed\n",
			ExitCode:   1,
			DurationMs: 500,
		},
	}
	input := sendRequest(t, "tools/call", 1, map[string]interface{}{
		"name":      "fusebox_test",
		"arguments": map[string]interface{}{},
	})
	lines := runServer(t, daemon, input)

	resp := parseResponse(t, lines[0])
	result, _ := json.Marshal(resp.Result)
	var r map[string]interface{}
	json.Unmarshal(result, &r)

	isError := r["isError"].(bool)
	if !isError {
		t.Fatal("expected isError=true for non-zero exit code")
	}
}

func TestToolsCall_DaemonUnreachable(t *testing.T) {
	daemon := &mockDaemon{
		execErr: fmt.Errorf("local machine unreachable: connection refused"),
	}
	input := sendRequest(t, "tools/call", 1, map[string]interface{}{
		"name":      "fusebox_build",
		"arguments": map[string]interface{}{},
	})
	lines := runServer(t, daemon, input)

	resp := parseResponse(t, lines[0])
	result, _ := json.Marshal(resp.Result)
	var r map[string]interface{}
	json.Unmarshal(result, &r)

	isError := r["isError"].(bool)
	if !isError {
		t.Fatal("expected isError=true for unreachable daemon")
	}
	content := r["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	if text != "local machine unreachable" {
		t.Fatalf("expected 'local machine unreachable', got: %s", text)
	}
}

func TestUnknownMethod(t *testing.T) {
	daemon := &mockDaemon{}
	input := sendRequest(t, "unknown/method", 1, nil)
	lines := runServer(t, daemon, input)

	resp := parseResponse(t, lines[0])
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Fatalf("expected error code -32601, got %d", resp.Error.Code)
	}
}

func TestMultipleRequests(t *testing.T) {
	daemon := &mockDaemon{
		actions: []ActionDescriptor{
			{Name: "build", Description: "Build"},
		},
		execResult: &ExecResult{Output: "ok", ExitCode: 0, DurationMs: 100},
	}

	lines := []string{
		sendRequest(t, "initialize", 1, nil),
		sendRequest(t, "tools/list", 2, nil),
		sendRequest(t, "tools/call", 3, map[string]interface{}{
			"name":      "fusebox_build",
			"arguments": map[string]interface{}{},
		}),
	}
	input := strings.Join(lines, "\n")
	responses := runServer(t, daemon, input)

	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}

	for i, line := range responses {
		resp := parseResponse(t, line)
		if resp.Error != nil {
			t.Fatalf("response %d had error: %v", i+1, resp.Error)
		}
	}
}

func TestNotificationsInitialized_NoResponse(t *testing.T) {
	daemon := &mockDaemon{}
	// notifications/initialized should produce no response.
	input := sendRequest(t, "notifications/initialized", 1, nil)
	in := strings.NewReader(input)
	var out bytes.Buffer
	srv := NewServer(daemon, in, &out)
	srv.Serve()

	if out.Len() != 0 {
		t.Fatalf("expected no response for notification, got: %s", out.String())
	}
}

func TestToolsCall_StripsFuseboxPrefix(t *testing.T) {
	var calledAction string
	daemon := &mockDaemon{
		execResult: &ExecResult{Output: "ok", ExitCode: 0, DurationMs: 10},
	}
	// Wrap to capture the action name.
	wrapper := &execCapture{daemon: daemon, calledAction: &calledAction}

	input := sendRequest(t, "tools/call", 1, map[string]interface{}{
		"name":      "fusebox_deploy",
		"arguments": map[string]interface{}{"env": "prod"},
	})
	runServer(t, wrapper, input)

	if calledAction != "deploy" {
		t.Fatalf("expected action 'deploy', got %q", calledAction)
	}
}

// execCapture wraps a mock to capture the action name passed to Exec.
type execCapture struct {
	daemon       *mockDaemon
	calledAction *string
}

func (e *execCapture) RequestActions() ([]ActionDescriptor, error) {
	return e.daemon.RequestActions()
}

func (e *execCapture) Exec(action string, params map[string]string) (*ExecResult, error) {
	*e.calledAction = action
	return e.daemon.Exec(action, params)
}

func (e *execCapture) Close() error { return nil }
