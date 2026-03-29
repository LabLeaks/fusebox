package daemon

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	gosync "sync"
	"testing"
	"time"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/rpc"
	fusesync "github.com/lableaks/fusebox/internal/sync"
)

func testConfig() *config.ProjectConfig {
	return &config.ProjectConfig{
		Version: 1,
		Actions: map[string]config.Action{
			"echo_test": {
				Description: "Echo a message",
				Exec:        "echo hello",
			},
			"param_test": {
				Description: "Echo with param",
				Exec:        "echo {msg}",
				Params: map[string]config.Param{
					"msg": {Type: "regex", Pattern: "^[a-z]+$"},
				},
			},
			"fail_test": {
				Description: "Exit with error",
				Exec:        "exit 42",
			},
		},
	}
}

func startTestServer(t *testing.T, secret string) (net.Conn, *Server) {
	t.Helper()

	serverConn, clientConn := net.Pipe()

	srv := NewServer(nil, ServerConfig{
		Config:     testConfig(),
		Secret:     secret,
		ProjectDir: os.TempDir(),
		Logger:     log.New(io.Discard, "", 0),
	})

	go srv.handleConn(serverConn)

	return clientConn, srv
}

func TestExecAction(t *testing.T) {
	secret := "test-secret-123"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	err := enc.Send(rpc.ExecRequest{
		Type:   rpc.TypeExec,
		Secret: secret,
		Action: "echo_test",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Read stdout messages until we get an exit
	var lines []string
	for {
		msgType, raw, err := dec.ReceiveEnvelope()
		if err != nil {
			t.Fatalf("ReceiveEnvelope: %v", err)
		}

		switch msgType {
		case rpc.TypeStdout:
			var msg rpc.StdoutMessage
			json.Unmarshal(raw, &msg)
			lines = append(lines, msg.Line)
		case rpc.TypeExit:
			var msg rpc.ExitMessage
			json.Unmarshal(raw, &msg)
			if msg.Code != 0 {
				t.Errorf("exit code = %d, want 0", msg.Code)
			}
			if msg.Duration <= 0 {
				t.Errorf("duration = %d, want > 0", msg.Duration)
			}
			goto done
		case rpc.TypeError:
			var msg rpc.ErrorResponse
			json.Unmarshal(raw, &msg)
			t.Fatalf("unexpected error: %s: %s", msg.Code, msg.Message)
		default:
			// stderr or other, continue
		}
	}
done:

	if len(lines) != 1 || lines[0] != "hello" {
		t.Errorf("stdout lines = %v, want [\"hello\"]", lines)
	}
}

func TestExecWithParams(t *testing.T) {
	secret := "test-secret-123"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	err := enc.Send(rpc.ExecRequest{
		Type:   rpc.TypeExec,
		Secret: secret,
		Action: "param_test",
		Params: map[string]string{"msg": "world"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var lines []string
	for {
		msgType, raw, err := dec.ReceiveEnvelope()
		if err != nil {
			t.Fatalf("ReceiveEnvelope: %v", err)
		}
		switch msgType {
		case rpc.TypeStdout:
			var msg rpc.StdoutMessage
			json.Unmarshal(raw, &msg)
			lines = append(lines, msg.Line)
		case rpc.TypeExit:
			goto done
		case rpc.TypeError:
			var msg rpc.ErrorResponse
			json.Unmarshal(raw, &msg)
			t.Fatalf("unexpected error: %s: %s", msg.Code, msg.Message)
		}
	}
done:
	if len(lines) != 1 || lines[0] != "world" {
		t.Errorf("stdout lines = %v, want [\"world\"]", lines)
	}
}

func TestExecNonZeroExit(t *testing.T) {
	secret := "test-secret-123"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(rpc.ExecRequest{
		Type:   rpc.TypeExec,
		Secret: secret,
		Action: "fail_test",
	})

	for {
		msgType, raw, err := dec.ReceiveEnvelope()
		if err != nil {
			t.Fatalf("ReceiveEnvelope: %v", err)
		}
		if msgType == rpc.TypeExit {
			var msg rpc.ExitMessage
			json.Unmarshal(raw, &msg)
			if msg.Code != 42 {
				t.Errorf("exit code = %d, want 42", msg.Code)
			}
			return
		}
	}
}

func TestExecInvalidAction(t *testing.T) {
	secret := "test-secret-123"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(rpc.ExecRequest{
		Type:   rpc.TypeExec,
		Secret: secret,
		Action: "nonexistent",
	})

	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeError {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeError)
	}

	var errMsg rpc.ErrorResponse
	json.Unmarshal(raw, &errMsg)
	if errMsg.Code != "INVALID_ACTION" {
		t.Errorf("error code = %q, want %q", errMsg.Code, "INVALID_ACTION")
	}
}

func TestExecInvalidParams(t *testing.T) {
	secret := "test-secret-123"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(rpc.ExecRequest{
		Type:   rpc.TypeExec,
		Secret: secret,
		Action: "param_test",
		Params: map[string]string{"msg": "INVALID_UPPER"},
	})

	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeError {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeError)
	}

	var errMsg rpc.ErrorResponse
	json.Unmarshal(raw, &errMsg)
	if errMsg.Code != "INVALID_PARAMS" {
		t.Errorf("error code = %q, want %q", errMsg.Code, "INVALID_PARAMS")
	}
}

func TestExecBadSecret(t *testing.T) {
	secret := "correct-secret"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(rpc.ExecRequest{
		Type:   rpc.TypeExec,
		Secret: "wrong-secret",
		Action: "echo_test",
	})

	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeError {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeError)
	}

	var errMsg rpc.ErrorResponse
	json.Unmarshal(raw, &errMsg)
	if errMsg.Code != "AUTH_ERROR" {
		t.Errorf("error code = %q, want %q", errMsg.Code, "AUTH_ERROR")
	}
}

func TestActionsRequest(t *testing.T) {
	secret := "test-secret-123"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(rpc.ActionsRequest{
		Type:   rpc.TypeActions,
		Secret: secret,
	})

	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeActionsResponse {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeActionsResponse)
	}

	var resp rpc.ActionsResponse
	json.Unmarshal(raw, &resp)

	if len(resp.Actions) != 3 {
		t.Fatalf("actions count = %d, want 3", len(resp.Actions))
	}

	// Actions should be sorted alphabetically
	if resp.Actions[0].Name != "echo_test" {
		t.Errorf("first action = %q, want %q", resp.Actions[0].Name, "echo_test")
	}
	if resp.Actions[1].Name != "fail_test" {
		t.Errorf("second action = %q, want %q", resp.Actions[1].Name, "fail_test")
	}
	if resp.Actions[2].Name != "param_test" {
		t.Errorf("third action = %q, want %q", resp.Actions[2].Name, "param_test")
	}

	// Check param schema is populated
	paramAction := resp.Actions[2]
	if paramAction.Params["msg"].Type != "regex" {
		t.Errorf("param type = %q, want %q", paramAction.Params["msg"].Type, "regex")
	}
	if paramAction.Params["msg"].Pattern != "^[a-z]+$" {
		t.Errorf("param pattern = %q, want %q", paramAction.Params["msg"].Pattern, "^[a-z]+$")
	}
}

func TestLastAction(t *testing.T) {
	secret := "test-secret-123"
	conn, srv := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	// Initially no last action
	if srv.GetLastAction() != nil {
		t.Fatal("expected nil last action initially")
	}

	enc.Send(rpc.ExecRequest{
		Type:   rpc.TypeExec,
		Secret: secret,
		Action: "echo_test",
	})

	// Drain until exit
	for {
		msgType, _, err := dec.ReceiveEnvelope()
		if err != nil {
			t.Fatalf("ReceiveEnvelope: %v", err)
		}
		if msgType == rpc.TypeExit {
			break
		}
	}

	// Give a moment for the server goroutine to record
	time.Sleep(10 * time.Millisecond)

	last := srv.GetLastAction()
	if last == nil {
		t.Fatal("expected last action to be recorded")
	}
	if last.Name != "echo_test" {
		t.Errorf("last action name = %q, want %q", last.Name, "echo_test")
	}
	if last.ExitCode != 0 {
		t.Errorf("last action exit code = %d, want 0", last.ExitCode)
	}
}

func TestStatusSocket(t *testing.T) {
	sockPath := t.TempDir() + "/test.sock"

	info := StatusInfo{
		Project:   "myproject",
		Server:    "spotless-2",
		Container: "running",
		SyncState: "watching",
	}

	statusSrv, err := NewStatusServer(sockPath, func() StatusInfo { return info })
	if err != nil {
		t.Fatalf("NewStatusServer: %v", err)
	}
	defer statusSrv.Close()

	go statusSrv.Serve()

	// Connect and read status
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	data, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var got StatusInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Project != "myproject" {
		t.Errorf("project = %q, want %q", got.Project, "myproject")
	}
	if got.Server != "spotless-2" {
		t.Errorf("server = %q, want %q", got.Server, "spotless-2")
	}
	if got.Container != "running" {
		t.Errorf("container = %q, want %q", got.Container, "running")
	}
}

func TestUnknownMessageType(t *testing.T) {
	secret := "test-secret-123"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(map[string]interface{}{
		"type":   "unknown_type",
		"secret": secret,
	})

	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeError {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeError)
	}

	var errMsg rpc.ErrorResponse
	json.Unmarshal(raw, &errMsg)
	if errMsg.Code != "UNKNOWN_TYPE" {
		t.Errorf("error code = %q, want %q", errMsg.Code, "UNKNOWN_TYPE")
	}
}

func TestActionsBadSecret(t *testing.T) {
	secret := "correct-secret"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(rpc.ActionsRequest{
		Type:   rpc.TypeActions,
		Secret: "wrong-secret",
	})

	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeError {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeError)
	}

	var errMsg rpc.ErrorResponse
	json.Unmarshal(raw, &errMsg)
	if errMsg.Code != "AUTH_ERROR" {
		t.Errorf("error code = %q, want %q", errMsg.Code, "AUTH_ERROR")
	}
}

func TestEmptyActionsConfig(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	srv := NewServer(nil, ServerConfig{
		Config:     &config.ProjectConfig{Version: 1, Actions: map[string]config.Action{}},
		Secret:     "s",
		ProjectDir: os.TempDir(),
		Logger:     log.New(io.Discard, "", 0),
	})
	go srv.handleConn(serverConn)

	enc := rpc.NewEncoder(clientConn)
	dec := rpc.NewDecoder(clientConn)

	enc.Send(rpc.ActionsRequest{Type: rpc.TypeActions, Secret: "s"})

	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeActionsResponse {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeActionsResponse)
	}

	var resp rpc.ActionsResponse
	json.Unmarshal(raw, &resp)

	if len(resp.Actions) != 0 {
		t.Errorf("actions count = %d, want 0", len(resp.Actions))
	}
}

func TestServeAndClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	srv := NewServer(listener, ServerConfig{
		Config:     testConfig(),
		Secret:     "s",
		ProjectDir: os.TempDir(),
		Logger:     log.New(io.Discard, "", 0),
	})

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(rpc.ExecRequest{Type: rpc.TypeExec, Secret: "s", Action: "echo_test"})

	for {
		msgType, _, err := dec.ReceiveEnvelope()
		if err != nil {
			t.Fatalf("ReceiveEnvelope: %v", err)
		}
		if msgType == rpc.TypeExit {
			break
		}
	}

	conn.Close()

	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

func TestConcurrentConnections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	srv := NewServer(listener, ServerConfig{
		Config:     testConfig(),
		Secret:     "s",
		ProjectDir: os.TempDir(),
		Logger:     log.New(io.Discard, "", 0),
	})

	go srv.Serve()
	defer srv.Close()

	const numClients = 5
	var wg gosync.WaitGroup
	errors := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				errors <- err
				return
			}
			defer conn.Close()

			enc := rpc.NewEncoder(conn)
			dec := rpc.NewDecoder(conn)

			if err := enc.Send(rpc.ExecRequest{
				Type: rpc.TypeExec, Secret: "s", Action: "echo_test",
			}); err != nil {
				errors <- err
				return
			}

			for {
				msgType, _, err := dec.ReceiveEnvelope()
				if err != nil {
					errors <- err
					return
				}
				if msgType == rpc.TypeExit {
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent client error: %v", err)
	}
}

func TestMultipleRequestsOnSameConnection(t *testing.T) {
	secret := "test-secret-123"
	conn, _ := startTestServer(t, secret)
	defer conn.Close()

	enc := rpc.NewEncoder(conn)
	dec := rpc.NewDecoder(conn)

	enc.Send(rpc.ExecRequest{Type: rpc.TypeExec, Secret: secret, Action: "echo_test"})
	for {
		msgType, _, err := dec.ReceiveEnvelope()
		if err != nil {
			t.Fatalf("first req: %v", err)
		}
		if msgType == rpc.TypeExit {
			break
		}
	}

	enc.Send(rpc.ActionsRequest{Type: rpc.TypeActions, Secret: secret})
	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("second req: %v", err)
	}
	if msgType != rpc.TypeActionsResponse {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeActionsResponse)
	}

	var resp rpc.ActionsResponse
	json.Unmarshal(raw, &resp)
	if len(resp.Actions) != 3 {
		t.Errorf("actions count = %d, want 3", len(resp.Actions))
	}
}

func TestStatusSocketMultipleConnections(t *testing.T) {
	// Use /tmp directly — t.TempDir() path + long test name exceeds macOS
	// Unix socket path limit (104 bytes).
	sockPath := "/tmp/fb-multi-sock-test.sock"
	defer os.Remove(sockPath)
	callCount := 0

	statusSrv, err := NewStatusServer(sockPath, func() StatusInfo {
		callCount++
		return StatusInfo{Project: "test"}
	})
	if err != nil {
		t.Fatalf("NewStatusServer: %v", err)
	}
	defer statusSrv.Close()

	go statusSrv.Serve()

	for i := 0; i < 3; i++ {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatalf("Dial %d: %v", i, err)
		}

		data, err := io.ReadAll(conn)
		conn.Close()
		if err != nil {
			t.Fatalf("ReadAll %d: %v", i, err)
		}

		var got StatusInfo
		json.Unmarshal(data, &got)
		if got.Project != "test" {
			t.Errorf("connection %d: project = %q, want %q", i, got.Project, "test")
		}
	}

	if callCount != 3 {
		t.Errorf("getInfo called %d times, want 3", callCount)
	}
}

func TestStatusSocketPath(t *testing.T) {
	path, err := StatusSocketPath("myproject")
	if err != nil {
		t.Fatalf("StatusSocketPath error: %v", err)
	}

	home, _ := os.UserHomeDir()
	want := home + "/.fusebox/run/myproject.sock"
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
}

// daemonMockSyncWaiter implements fusesync.SyncWaiter for testing sync-wait in daemon.
type daemonMockSyncWaiter struct {
	state   fusesync.SyncState
	waitErr error
}

func (w *daemonMockSyncWaiter) State(sessionName string) (fusesync.SyncState, error) {
	return w.state, nil
}

func (w *daemonMockSyncWaiter) WaitForWatching(sessionName string, timeout time.Duration) (fusesync.SyncState, error) {
	if w.waitErr != nil {
		return w.state, w.waitErr
	}
	return fusesync.StateWatching, nil
}

func TestExecWithSyncWait(t *testing.T) {
	waiter := &daemonMockSyncWaiter{state: fusesync.StateWatching}

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	srv := NewServer(nil, ServerConfig{
		Config:      testConfig(),
		Secret:      "s",
		ProjectDir:  os.TempDir(),
		Logger:      log.New(io.Discard, "", 0),
		SyncWaiter:  waiter,
		SessionName: "fusebox-src-test",
		SyncTimeout: 5 * time.Second,
	})
	go srv.handleConn(serverConn)

	enc := rpc.NewEncoder(clientConn)
	dec := rpc.NewDecoder(clientConn)

	enc.Send(rpc.ExecRequest{Type: rpc.TypeExec, Secret: "s", Action: "echo_test"})

	for {
		msgType, raw, err := dec.ReceiveEnvelope()
		if err != nil {
			t.Fatalf("ReceiveEnvelope: %v", err)
		}
		if msgType == rpc.TypeExit {
			var msg rpc.ExitMessage
			json.Unmarshal(raw, &msg)
			if msg.Code != 0 {
				t.Errorf("exit code = %d, want 0", msg.Code)
			}
			return
		}
	}
}

func TestNewServerDefaultSyncTimeout(t *testing.T) {
	srv := NewServer(nil, ServerConfig{
		Config:     testConfig(),
		Secret:     "s",
		ProjectDir: os.TempDir(),
		Logger:     log.New(io.Discard, "", 0),
	})

	if srv.syncTimeout != 30*time.Second {
		t.Errorf("syncTimeout = %v, want 30s", srv.syncTimeout)
	}
}

func TestNewServerCustomSyncTimeout(t *testing.T) {
	srv := NewServer(nil, ServerConfig{
		Config:      testConfig(),
		Secret:      "s",
		ProjectDir:  os.TempDir(),
		Logger:      log.New(io.Discard, "", 0),
		SyncTimeout: 10 * time.Second,
	})

	if srv.syncTimeout != 10*time.Second {
		t.Errorf("syncTimeout = %v, want 10s", srv.syncTimeout)
	}
}
