package integration

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lableaks/fusebox/internal/rpc"
)

// recorder implements rpc.StreamHandler and captures all callbacks.
type recorder struct {
	stdout []string
	stderr []string
	exits  []exitInfo
	errors []errorInfo
}

type exitInfo struct {
	Code       int
	DurationMs int64
}

type errorInfo struct {
	Code    string
	Message string
}

func (r *recorder) OnStdout(line string)         { r.stdout = append(r.stdout, line) }
func (r *recorder) OnStderr(line string)         { r.stderr = append(r.stderr, line) }
func (r *recorder) OnExit(code int, dur int64)   { r.exits = append(r.exits, exitInfo{code, dur}) }
func (r *recorder) OnError(code, message string) { r.errors = append(r.errors, errorInfo{code, message}) }

func TestExecBuildAction(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	rec := &recorder{}
	err = client.ExecStream("build", nil, io.Discard, rec)
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}

	if len(rec.exits) != 1 {
		t.Fatalf("expected 1 exit, got %d", len(rec.exits))
	}
	if rec.exits[0].Code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %v", rec.exits[0].Code, rec.stderr)
	}
	if rec.exits[0].DurationMs <= 0 {
		t.Errorf("duration = %d, want > 0", rec.exits[0].DurationMs)
	}
}

func TestExecEchoWithParams(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	rec := &recorder{}
	params := map[string]string{
		"prefix":  "INFO",
		"message": "hello world",
	}
	err = client.ExecStream("echo", params, io.Discard, rec)
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}

	if len(rec.exits) != 1 || rec.exits[0].Code != 0 {
		t.Fatalf("expected exit 0, got %v", rec.exits)
	}

	// stdout should contain the echoed message
	joined := strings.Join(rec.stdout, "\n")
	if !strings.Contains(joined, "INFO") || !strings.Contains(joined, "hello world") {
		t.Errorf("stdout = %q, want to contain 'INFO' and 'hello world'", joined)
	}
}

func TestExecCountWithIntParam(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	rec := &recorder{}
	err = client.ExecStream("count", map[string]string{"n": "5"}, io.Discard, rec)
	if err != nil {
		t.Fatalf("ExecStream: %v", err)
	}

	if len(rec.exits) != 1 || rec.exits[0].Code != 0 {
		t.Fatalf("expected exit 0, got %v", rec.exits)
	}
	if len(rec.stdout) != 5 {
		t.Errorf("stdout lines = %d, want 5; got %v", len(rec.stdout), rec.stdout)
	}
	if len(rec.stdout) > 0 && rec.stdout[0] != "1" {
		t.Errorf("first line = %q, want %q", rec.stdout[0], "1")
	}
	if len(rec.stdout) >= 5 && rec.stdout[4] != "5" {
		t.Errorf("last line = %q, want %q", rec.stdout[4], "5")
	}
}

func TestExecInvalidParams(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	rec := &recorder{}
	// Invalid: message contains special chars that don't match the regex
	params := map[string]string{
		"prefix":  "INFO",
		"message": "hello!@#$",
	}
	err = client.ExecStream("echo", params, io.Discard, rec)
	if err == nil {
		t.Fatal("expected error for invalid params")
	}
	if !strings.Contains(err.Error(), "INVALID_PARAMS") {
		t.Errorf("error = %q, want to contain 'INVALID_PARAMS'", err.Error())
	}
}

func TestExecMissingParams(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	rec := &recorder{}
	// Missing 'message' param
	err = client.ExecStream("echo", map[string]string{"prefix": "INFO"}, io.Discard, rec)
	if err == nil {
		t.Fatal("expected error for missing params")
	}
	if !strings.Contains(err.Error(), "INVALID_PARAMS") {
		t.Errorf("error = %q, want to contain 'INVALID_PARAMS'", err.Error())
	}
}

func TestExecUnknownAction(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	rec := &recorder{}
	err = client.ExecStream("nonexistent", nil, io.Discard, rec)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), "INVALID_ACTION") {
		t.Errorf("error = %q, want to contain 'INVALID_ACTION'", err.Error())
	}
}

func TestExecIntParamOutOfRange(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	rec := &recorder{}
	err = client.ExecStream("count", map[string]string{"n": "999"}, io.Discard, rec)
	if err == nil {
		t.Fatal("expected error for out-of-range int")
	}
	if !strings.Contains(err.Error(), "INVALID_PARAMS") {
		t.Errorf("error = %q, want to contain 'INVALID_PARAMS'", err.Error())
	}
}

func TestExecMultipleRequestsSameConnection(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// First request
	rec1 := &recorder{}
	err = client.ExecStream("count", map[string]string{"n": "3"}, io.Discard, rec1)
	if err != nil {
		t.Fatalf("first exec: %v", err)
	}
	if len(rec1.stdout) != 3 {
		t.Errorf("first exec: stdout lines = %d, want 3", len(rec1.stdout))
	}

	// Second request on same connection
	rec2 := &recorder{}
	err = client.ExecStream("count", map[string]string{"n": "2"}, io.Discard, rec2)
	if err != nil {
		t.Fatalf("second exec: %v", err)
	}
	if len(rec2.stdout) != 2 {
		t.Errorf("second exec: stdout lines = %d, want 2", len(rec2.stdout))
	}
}

func TestRequestActions(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, secret, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	resp, err := client.RequestActions()
	if err != nil {
		t.Fatalf("RequestActions: %v", err)
	}

	if len(resp.Actions) != 3 {
		t.Fatalf("actions count = %d, want 3", len(resp.Actions))
	}

	// Actions should be sorted by name
	names := make([]string, len(resp.Actions))
	for i, a := range resp.Actions {
		names[i] = a.Name
	}
	want := []string{"build", "count", "echo"}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("actions[%d] = %q, want %q", i, names[i], n)
		}
	}

	// Verify echo action has params
	var echoAction *rpc.ActionInfo
	for i := range resp.Actions {
		if resp.Actions[i].Name == "echo" {
			echoAction = &resp.Actions[i]
			break
		}
	}
	if echoAction == nil {
		t.Fatal("echo action not found")
	}
	if len(echoAction.Params) != 2 {
		t.Errorf("echo params count = %d, want 2", len(echoAction.Params))
	}
	if echoAction.Params["prefix"].Type != "enum" {
		t.Errorf("prefix type = %q, want %q", echoAction.Params["prefix"].Type, "enum")
	}
}

func TestConnectionRefused(t *testing.T) {
	_, err := rpc.Dial(rpc.ClientConfig{
		Address: "127.0.0.1:1", // port 1 should never be listening
		Secret:  "test",
	})
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
	if !strings.Contains(err.Error(), "local machine unreachable") {
		t.Errorf("error = %q, want to contain 'local machine unreachable'", err.Error())
	}
}

func TestDialTimeout(t *testing.T) {
	// Use a non-routable address to trigger timeout
	start := time.Now()
	_, err := rpc.Dial(rpc.ClientConfig{
		Address: "192.0.2.1:9999", // RFC 5737 TEST-NET-1, non-routable
		Secret:  "test",
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
	// Should timeout within ~10s (the Dial timeout)
	if elapsed > 15*time.Second {
		t.Errorf("took %v, expected < 15s", elapsed)
	}
}
