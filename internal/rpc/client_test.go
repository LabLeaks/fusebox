package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	fusesync "github.com/lableaks/fusebox/internal/sync"
)

// mockStreamHandler records callbacks from ExecStream.
type mockStreamHandler struct {
	stdout []string
	stderr []string
	exits  []exitRecord
	errors []errorRecord
}

type exitRecord struct {
	Code       int
	DurationMs int64
}

type errorRecord struct {
	Code    string
	Message string
}

func (h *mockStreamHandler) OnStdout(line string)          { h.stdout = append(h.stdout, line) }
func (h *mockStreamHandler) OnStderr(line string)          { h.stderr = append(h.stderr, line) }
func (h *mockStreamHandler) OnExit(code int, dur int64)    { h.exits = append(h.exits, exitRecord{code, dur}) }
func (h *mockStreamHandler) OnError(code, message string)  { h.errors = append(h.errors, errorRecord{code, message}) }

// mockSyncWaiter implements fusesync.SyncWaiter for testing.
type mockSyncWaiter struct {
	state    fusesync.SyncState
	stateErr error
	waitErr  error
	called   bool
}

func (w *mockSyncWaiter) State(sessionName string) (fusesync.SyncState, error) {
	return w.state, w.stateErr
}

func (w *mockSyncWaiter) WaitForWatching(sessionName string, timeout time.Duration) (fusesync.SyncState, error) {
	w.called = true
	if w.waitErr != nil {
		return w.state, w.waitErr
	}
	return fusesync.StateWatching, nil
}

// pipe creates a connected pair of buffers simulating a client-server connection.
// serverWrite is what the "server" sends to the client; clientWrite captures what
// the client sends to the server.
type testConn struct {
	reader *bytes.Buffer // data client reads (server responses)
	writer *bytes.Buffer // data client writes (client requests)
}

func (tc *testConn) Read(p []byte) (int, error)  { return tc.reader.Read(p) }
func (tc *testConn) Write(p []byte) (int, error) { return tc.writer.Write(p) }

func newTestConn(serverResponses string) *testConn {
	return &testConn{
		reader: bytes.NewBufferString(serverResponses),
		writer: &bytes.Buffer{},
	}
}

func marshalLine(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data) + "\n"
}

// --- Tests ---

func TestClient_ExecStream_StdoutStderrExit(t *testing.T) {
	responses := marshalLine(StdoutMessage{Type: TypeStdout, Secret: "s", Line: "hello"}) +
		marshalLine(StderrMessage{Type: TypeStderr, Secret: "s", Line: "warn"}) +
		marshalLine(StdoutMessage{Type: TypeStdout, Secret: "s", Line: "world"}) +
		marshalLine(ExitMessage{Type: TypeExit, Secret: "s", Code: 0, Duration: 150})

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "s"})
	handler := &mockStreamHandler{}

	err := client.ExecStream("build", nil, io.Discard, handler)
	if err != nil {
		t.Fatalf("ExecStream error: %v", err)
	}

	if len(handler.stdout) != 2 || handler.stdout[0] != "hello" || handler.stdout[1] != "world" {
		t.Errorf("stdout = %v, want [hello world]", handler.stdout)
	}
	if len(handler.stderr) != 1 || handler.stderr[0] != "warn" {
		t.Errorf("stderr = %v, want [warn]", handler.stderr)
	}
	if len(handler.exits) != 1 || handler.exits[0].Code != 0 {
		t.Errorf("exits = %v, want [{0 150}]", handler.exits)
	}
}

func TestClient_ExecStream_NonZeroExit(t *testing.T) {
	responses := marshalLine(ExitMessage{Type: TypeExit, Secret: "s", Code: 1, Duration: 50})

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "s"})
	handler := &mockStreamHandler{}

	err := client.ExecStream("test", nil, io.Discard, handler)
	if err != nil {
		t.Fatalf("ExecStream error: %v", err)
	}
	if handler.exits[0].Code != 1 {
		t.Errorf("exit code = %d, want 1", handler.exits[0].Code)
	}
}

func TestClient_ExecStream_ErrorResponse(t *testing.T) {
	responses := marshalLine(ErrorResponse{Type: TypeError, Secret: "s", Code: "unknown_action", Message: "no action 'foo'"})

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "s"})
	handler := &mockStreamHandler{}

	err := client.ExecStream("foo", nil, io.Discard, handler)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown_action") {
		t.Errorf("error = %q, want to contain 'unknown_action'", err.Error())
	}
	if len(handler.errors) != 1 {
		t.Errorf("errors = %v, want 1 error", handler.errors)
	}
}

func TestClient_ExecStream_ConnectionClosed(t *testing.T) {
	conn := newTestConn("") // empty: EOF immediately
	client := NewClient(conn, ClientConfig{Secret: "s"})
	handler := &mockStreamHandler{}

	err := client.ExecStream("build", nil, io.Discard, handler)
	if err == nil {
		t.Fatal("expected error on EOF")
	}
	if !strings.Contains(err.Error(), "connection closed") {
		t.Errorf("error = %q, want to contain 'connection closed'", err.Error())
	}
}

func TestClient_ExecStream_SendsCorrectRequest(t *testing.T) {
	responses := marshalLine(ExitMessage{Type: TypeExit, Secret: "s", Code: 0, Duration: 10})

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "my-secret"})
	handler := &mockStreamHandler{}

	params := map[string]string{"env": "staging", "replicas": "3"}
	_ = client.ExecStream("deploy", params, io.Discard, handler)

	var req ExecRequest
	if err := json.Unmarshal(conn.writer.Bytes(), &req); err != nil {
		t.Fatalf("unmarshaling sent request: %v", err)
	}
	if req.Type != TypeExec {
		t.Errorf("type = %q, want %q", req.Type, TypeExec)
	}
	if req.Secret != "my-secret" {
		t.Errorf("secret = %q, want %q", req.Secret, "my-secret")
	}
	if req.Action != "deploy" {
		t.Errorf("action = %q, want %q", req.Action, "deploy")
	}
	if req.Params["env"] != "staging" || req.Params["replicas"] != "3" {
		t.Errorf("params = %v", req.Params)
	}
}

func TestClient_ExecStream_SyncWaitCalled(t *testing.T) {
	responses := marshalLine(ExitMessage{Type: TypeExit, Secret: "s", Code: 0, Duration: 10})

	waiter := &mockSyncWaiter{state: fusesync.StateWatching}
	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{
		Secret:      "s",
		SyncWaiter:  waiter,
		SessionName: "fusebox-src-myapp",
		SyncTimeout: 5 * time.Second,
	})
	handler := &mockStreamHandler{}

	var logBuf bytes.Buffer
	err := client.ExecStream("build", nil, &logBuf, handler)
	if err != nil {
		t.Fatalf("ExecStream error: %v", err)
	}

	// State was already Watching, so WaitForWatching should not be called
	// (WaitForSyncWithLog checks State first and short-circuits)
	// But the SyncWaiter.State() was called
	if logBuf.Len() != 0 {
		t.Errorf("expected no sync log when already watching, got %q", logBuf.String())
	}
}

func TestClient_ExecStream_NoSyncWaitWithoutWaiter(t *testing.T) {
	responses := marshalLine(ExitMessage{Type: TypeExit, Secret: "s", Code: 0, Duration: 10})

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "s"}) // no SyncWaiter
	handler := &mockStreamHandler{}

	err := client.ExecStream("build", nil, io.Discard, handler)
	if err != nil {
		t.Fatalf("ExecStream error: %v", err)
	}
	// Should succeed without sync-wait
}

func TestClient_RequestActions(t *testing.T) {
	min, max := 1, 10
	resp := ActionsResponse{
		Type:   TypeActionsResponse,
		Secret: "s",
		Actions: []ActionInfo{
			{Name: "build", Description: "Build project"},
			{Name: "deploy", Description: "Deploy", Params: map[string]ParamSchema{
				"replicas": {Type: "int", Min: &min, Max: &max},
			}},
		},
	}
	responses := marshalLine(resp)

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "s"})

	got, err := client.RequestActions()
	if err != nil {
		t.Fatalf("RequestActions error: %v", err)
	}
	if len(got.Actions) != 2 {
		t.Fatalf("actions count = %d, want 2", len(got.Actions))
	}
	if got.Actions[0].Name != "build" {
		t.Errorf("actions[0].name = %q, want %q", got.Actions[0].Name, "build")
	}
	if got.Actions[1].Params["replicas"].Type != "int" {
		t.Errorf("actions[1].params.replicas.type = %q, want %q", got.Actions[1].Params["replicas"].Type, "int")
	}
}

func TestClient_RequestActions_ErrorResponse(t *testing.T) {
	responses := marshalLine(ErrorResponse{Type: TypeError, Secret: "s", Code: "auth_failed", Message: "invalid secret"})

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "wrong"})

	_, err := client.RequestActions()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "auth_failed") {
		t.Errorf("error = %q, want to contain 'auth_failed'", err.Error())
	}
}

func TestClient_RequestActions_UnexpectedType(t *testing.T) {
	responses := marshalLine(StdoutMessage{Type: TypeStdout, Secret: "s", Line: "unexpected"})

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "s"})

	_, err := client.RequestActions()
	if err == nil {
		t.Fatal("expected error for unexpected message type")
	}
	if !strings.Contains(err.Error(), "unexpected message type") {
		t.Errorf("error = %q, want to contain 'unexpected message type'", err.Error())
	}
}

func TestClient_RequestActions_EmptyResponse(t *testing.T) {
	conn := newTestConn("")
	client := NewClient(conn, ClientConfig{Secret: "s"})

	_, err := client.RequestActions()
	if err == nil {
		t.Fatal("expected error on EOF")
	}
}

func TestClient_ExecStream_SyncWaitFails(t *testing.T) {
	waiter := &mockSyncWaiter{
		state:    fusesync.StateSyncing,
		stateErr: fmt.Errorf("connection refused"),
	}

	responses := marshalLine(ExitMessage{Type: TypeExit, Secret: "s", Code: 0, Duration: 10})
	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{
		Secret:      "s",
		SyncWaiter:  waiter,
		SessionName: "fusebox-src-test",
		SyncTimeout: 1 * time.Second,
	})

	handler := &mockStreamHandler{}
	err := client.ExecStream("build", nil, io.Discard, handler)
	if err == nil {
		t.Fatal("expected error when sync-wait fails")
	}
	if !strings.Contains(err.Error(), "sync-wait") {
		t.Errorf("error = %q, want to contain 'sync-wait'", err.Error())
	}
}

func TestClient_ExecStream_UnexpectedMessageType(t *testing.T) {
	responses := marshalLine(map[string]interface{}{
		"type":   "some_weird_type",
		"secret": "s",
	})

	conn := newTestConn(responses)
	client := NewClient(conn, ClientConfig{Secret: "s"})
	handler := &mockStreamHandler{}

	err := client.ExecStream("build", nil, io.Discard, handler)
	if err == nil {
		t.Fatal("expected error for unexpected message type")
	}
	if !strings.Contains(err.Error(), "unexpected message type") {
		t.Errorf("error = %q, want to contain 'unexpected message type'", err.Error())
	}
}

func TestClient_Close_NilConn(t *testing.T) {
	client := NewClient(&testConn{
		reader: bytes.NewBuffer(nil),
		writer: &bytes.Buffer{},
	}, ClientConfig{Secret: "s"})

	// NewClient doesn't set conn field, so Close should handle nil
	err := client.Close()
	if err != nil {
		t.Errorf("Close nil conn error: %v", err)
	}
}

func TestClient_DefaultSyncTimeout(t *testing.T) {
	conn := newTestConn("")
	client := NewClient(conn, ClientConfig{Secret: "s"})

	if client.cfg.SyncTimeout != 30*time.Second {
		t.Errorf("SyncTimeout = %v, want 30s", client.cfg.SyncTimeout)
	}
}
