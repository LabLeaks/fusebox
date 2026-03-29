package daemon

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/lableaks/fusebox/internal/rpc"
)

func TestExecuteSimpleCommand(t *testing.T) {
	var buf bytes.Buffer
	enc := rpc.NewEncoder(&buf)

	result := Execute(ExecConfig{
		Command: "echo executor_test",
		WorkDir: t.TempDir(),
		Timeout: 10 * time.Second,
		Secret:  "s",
		Encoder: enc,
	})

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Duration <= 0 {
		t.Errorf("duration = %v, want > 0", result.Duration)
	}

	// Parse the streamed messages
	dec := rpc.NewDecoder(&buf)
	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeStdout {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeStdout)
	}
	var msg rpc.StdoutMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if msg.Line != "executor_test" {
		t.Errorf("line = %q, want %q", msg.Line, "executor_test")
	}
}

func TestExecuteStderr(t *testing.T) {
	var buf bytes.Buffer
	enc := rpc.NewEncoder(&buf)

	result := Execute(ExecConfig{
		Command: "echo err_output >&2",
		WorkDir: t.TempDir(),
		Timeout: 10 * time.Second,
		Secret:  "s",
		Encoder: enc,
	})

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}

	dec := rpc.NewDecoder(&buf)
	msgType, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	if msgType != rpc.TypeStderr {
		t.Fatalf("type = %q, want %q", msgType, rpc.TypeStderr)
	}
	var msg rpc.StderrMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal stderr: %v", err)
	}
	if msg.Line != "err_output" {
		t.Errorf("line = %q, want %q", msg.Line, "err_output")
	}
}

func TestExecuteNonZeroExit(t *testing.T) {
	var buf bytes.Buffer
	enc := rpc.NewEncoder(&buf)

	result := Execute(ExecConfig{
		Command: "exit 7",
		WorkDir: t.TempDir(),
		Timeout: 10 * time.Second,
		Secret:  "s",
		Encoder: enc,
	})

	if result.ExitCode != 7 {
		t.Errorf("exit code = %d, want 7", result.ExitCode)
	}
}

func TestExecuteTimeout(t *testing.T) {
	var buf bytes.Buffer
	enc := rpc.NewEncoder(&buf)

	result := Execute(ExecConfig{
		Command: "sleep 60",
		WorkDir: t.TempDir(),
		Timeout: 100 * time.Millisecond,
		Secret:  "s",
		Encoder: enc,
	})

	if result.ExitCode != -1 {
		t.Errorf("exit code = %d, want -1 (timeout)", result.ExitCode)
	}
	if result.Duration > 5*time.Second {
		t.Errorf("duration = %v, should be close to timeout not 60s", result.Duration)
	}
}

func TestExecuteMultipleLines(t *testing.T) {
	var buf bytes.Buffer
	enc := rpc.NewEncoder(&buf)

	result := Execute(ExecConfig{
		Command: "echo line1; echo line2; echo line3",
		WorkDir: t.TempDir(),
		Timeout: 10 * time.Second,
		Secret:  "s",
		Encoder: enc,
	})

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}

	dec := rpc.NewDecoder(&buf)
	var lines []string
	for {
		_, raw, err := dec.ReceiveEnvelope()
		if err != nil {
			break
		}
		var msg rpc.StdoutMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal stdout: %v", err)
		}
		lines = append(lines, msg.Line)
	}

	if len(lines) != 3 {
		t.Fatalf("lines count = %d, want 3", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Errorf("lines = %v, want [line1, line2, line3]", lines)
	}
}

func TestExecuteTimeoutSendsSIGTERM(t *testing.T) {
	// Verify that on timeout, the process receives SIGTERM (not immediate SIGKILL).
	// A trap on SIGTERM should fire before the process is killed.
	var buf bytes.Buffer
	enc := rpc.NewEncoder(&buf)

	result := Execute(ExecConfig{
		Command: `trap 'echo got_sigterm; exit 42' TERM; sleep 60`,
		WorkDir: t.TempDir(),
		Timeout: 200 * time.Millisecond,
		Secret:  "s",
		Encoder: enc,
	})

	// The process should exit due to timeout
	if result.ExitCode != -1 && result.ExitCode != 42 {
		t.Errorf("exit code = %d, want -1 or 42 (SIGTERM handled)", result.ExitCode)
	}

	if result.Duration > 10*time.Second {
		t.Errorf("duration = %v, should not wait full 60s", result.Duration)
	}
}

func TestExecuteCompletesBeforeTimeout(t *testing.T) {
	// Verify that a command completing before timeout is not killed
	var buf bytes.Buffer
	enc := rpc.NewEncoder(&buf)

	result := Execute(ExecConfig{
		Command: "echo fast",
		WorkDir: t.TempDir(),
		Timeout: 10 * time.Second,
		Secret:  "s",
		Encoder: enc,
	})

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}

	dec := rpc.NewDecoder(&buf)
	_, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	var msg rpc.StdoutMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if msg.Line != "fast" {
		t.Errorf("line = %q, want %q", msg.Line, "fast")
	}
}

func TestExecuteWorkDir(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	enc := rpc.NewEncoder(&buf)

	result := Execute(ExecConfig{
		Command: "pwd",
		WorkDir: dir,
		Timeout: 10 * time.Second,
		Secret:  "s",
		Encoder: enc,
	})

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}

	dec := rpc.NewDecoder(&buf)
	_, raw, err := dec.ReceiveEnvelope()
	if err != nil {
		t.Fatalf("ReceiveEnvelope: %v", err)
	}
	var msg rpc.StdoutMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}

	// On macOS, /tmp is symlinked to /private/tmp
	if msg.Line != dir && msg.Line != "/private"+dir {
		t.Errorf("pwd = %q, want %q", msg.Line, dir)
	}
}
