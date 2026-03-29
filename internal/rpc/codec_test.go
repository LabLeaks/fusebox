package rpc

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestRoundtripExecRequest(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := ExecRequest{
		Type:   TypeExec,
		Secret: "abc123",
		Action: "build",
		Params: map[string]string{"target": "linux"},
	}

	if err := enc.Send(sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got ExecRequest
	if err := dec.Receive(&got); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if got.Type != TypeExec {
		t.Errorf("type = %q, want %q", got.Type, TypeExec)
	}
	if got.Action != "build" {
		t.Errorf("action = %q, want %q", got.Action, "build")
	}
	if got.Params["target"] != "linux" {
		t.Errorf("params[target] = %q, want %q", got.Params["target"], "linux")
	}
}

func TestRoundtripStdoutMessage(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := StdoutMessage{Type: TypeStdout, Secret: "s", Line: "hello world"}
	if err := enc.Send(sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got StdoutMessage
	if err := dec.Receive(&got); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if got.Type != TypeStdout {
		t.Errorf("type = %q, want %q", got.Type, TypeStdout)
	}
	if got.Line != "hello world" {
		t.Errorf("line = %q, want %q", got.Line, "hello world")
	}
}

func TestRoundtripStderrMessage(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := StderrMessage{Type: TypeStderr, Secret: "s", Line: "error: bad input"}
	if err := enc.Send(sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got StderrMessage
	if err := dec.Receive(&got); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if got.Type != TypeStderr {
		t.Errorf("type = %q, want %q", got.Type, TypeStderr)
	}
	if got.Line != "error: bad input" {
		t.Errorf("line = %q, want %q", got.Line, "error: bad input")
	}
}

func TestRoundtripExitMessage(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := ExitMessage{Type: TypeExit, Secret: "s", Code: 1, Duration: 1500}
	if err := enc.Send(sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got ExitMessage
	if err := dec.Receive(&got); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if got.Type != TypeExit {
		t.Errorf("type = %q, want %q", got.Type, TypeExit)
	}
	if got.Code != 1 {
		t.Errorf("code = %d, want 1", got.Code)
	}
	if got.Duration != 1500 {
		t.Errorf("duration = %d, want 1500", got.Duration)
	}
}

func TestRoundtripActionsRequest(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := ActionsRequest{Type: TypeActions, Secret: "s"}
	if err := enc.Send(sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got ActionsRequest
	if err := dec.Receive(&got); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if got.Type != TypeActions {
		t.Errorf("type = %q, want %q", got.Type, TypeActions)
	}
}

func TestRoundtripActionsResponse(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	minVal := 1
	maxVal := 100
	sent := ActionsResponse{
		Type:   TypeActionsResponse,
		Secret: "s",
		Actions: []ActionInfo{
			{
				Name:        "build",
				Description: "Run build",
				Params: map[string]ParamSchema{
					"target": {Type: "enum", Values: []string{"linux", "darwin"}},
					"jobs":   {Type: "int", Min: &minVal, Max: &maxVal},
				},
			},
		},
	}
	if err := enc.Send(sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got ActionsResponse
	if err := dec.Receive(&got); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if got.Type != TypeActionsResponse {
		t.Errorf("type = %q, want %q", got.Type, TypeActionsResponse)
	}
	if len(got.Actions) != 1 {
		t.Fatalf("actions count = %d, want 1", len(got.Actions))
	}
	if got.Actions[0].Name != "build" {
		t.Errorf("action name = %q, want %q", got.Actions[0].Name, "build")
	}
	if got.Actions[0].Params["target"].Type != "enum" {
		t.Errorf("param type = %q, want %q", got.Actions[0].Params["target"].Type, "enum")
	}
	if *got.Actions[0].Params["jobs"].Min != 1 {
		t.Errorf("param min = %d, want 1", *got.Actions[0].Params["jobs"].Min)
	}
}

func TestRoundtripErrorResponse(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	sent := ErrorResponse{
		Type:    TypeError,
		Secret:  "s",
		Code:    "INVALID_ACTION",
		Message: "unknown action: deploy",
	}
	if err := enc.Send(sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got ErrorResponse
	if err := dec.Receive(&got); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if got.Type != TypeError {
		t.Errorf("type = %q, want %q", got.Type, TypeError)
	}
	if got.Code != "INVALID_ACTION" {
		t.Errorf("code = %q, want %q", got.Code, "INVALID_ACTION")
	}
	if got.Message != "unknown action: deploy" {
		t.Errorf("message = %q, want %q", got.Message, "unknown action: deploy")
	}
}

func TestReceiveEnvelope(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	// Send two different message types
	if err := enc.Send(ExecRequest{Type: TypeExec, Secret: "s", Action: "test"}); err != nil {
		t.Fatalf("Send exec: %v", err)
	}
	if err := enc.Send(StdoutMessage{Type: TypeStdout, Secret: "s", Line: "ok"}); err != nil {
		t.Fatalf("Send stdout: %v", err)
	}

	// First message
	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope 1: %v", err)
	}
	if msgType != TypeExec {
		t.Errorf("type = %q, want %q", msgType, TypeExec)
	}
	var exec ExecRequest
	if err := json.Unmarshal(raw, &exec); err != nil {
		t.Fatalf("Unmarshal exec: %v", err)
	}
	if exec.Action != "test" {
		t.Errorf("action = %q, want %q", exec.Action, "test")
	}

	// Second message
	msgType, raw, err = dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope 2: %v", err)
	}
	if msgType != TypeStdout {
		t.Errorf("type = %q, want %q", msgType, TypeStdout)
	}
	var stdout StdoutMessage
	if err := json.Unmarshal(raw, &stdout); err != nil {
		t.Fatalf("Unmarshal stdout: %v", err)
	}
	if stdout.Line != "ok" {
		t.Errorf("line = %q, want %q", stdout.Line, "ok")
	}
}

func TestReceiveEOF(t *testing.T) {
	var buf bytes.Buffer
	dec := NewDecoder(&buf)

	var msg ExecRequest
	err := dec.Receive(&msg)
	if err != io.EOF {
		t.Errorf("err = %v, want io.EOF", err)
	}
}

func TestReceiveEnvelopeEOF(t *testing.T) {
	var buf bytes.Buffer
	dec := NewDecoder(&buf)

	_, _, err := dec.ReceiveEnvelope()
	if err != io.EOF {
		t.Errorf("err = %v, want io.EOF", err)
	}
}

func TestReceiveInvalidJSON(t *testing.T) {
	buf := bytes.NewBufferString("not json\n")
	dec := NewDecoder(buf)

	var msg ExecRequest
	err := dec.Receive(&msg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	for i := 0; i < 100; i++ {
		msg := StdoutMessage{Type: TypeStdout, Secret: "s", Line: "line"}
		if err := enc.Send(msg); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}

	for i := 0; i < 100; i++ {
		var got StdoutMessage
		if err := dec.Receive(&got); err != nil {
			t.Fatalf("Receive %d: %v", i, err)
		}
		if got.Line != "line" {
			t.Errorf("message %d: line = %q, want %q", i, got.Line, "line")
		}
	}
}
