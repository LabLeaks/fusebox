package daemon

import (
	"bufio"
	"os/exec"
	"syscall"
	"time"

	"github.com/lableaks/fusebox/internal/rpc"
)

// ExecConfig configures a command execution.
type ExecConfig struct {
	Command string
	WorkDir string
	Timeout time.Duration
	Secret  string
	Encoder *rpc.Encoder
}

// ExecResult holds the outcome of an execution.
type ExecResult struct {
	ExitCode int
	Duration time.Duration
}

// Execute runs a shell command, streaming stdout/stderr as RPC messages.
// On timeout, sends SIGTERM to the process group, then SIGKILL after 5s grace.
func Execute(cfg ExecConfig) ExecResult {
	start := time.Now()

	cmd := exec.Command("sh", "-c", cfg.Command)
	cmd.Dir = cfg.WorkDir
	// Use process group so we can kill child processes
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ExecResult{ExitCode: -1, Duration: time.Since(start)}
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return ExecResult{ExitCode: -1, Duration: time.Since(start)}
	}

	if err := cmd.Start(); err != nil {
		return ExecResult{ExitCode: -1, Duration: time.Since(start)}
	}

	// Set up timeout: SIGTERM then SIGKILL after 5s grace period
	timedOut := make(chan struct{})
	var killTimer *time.Timer
	timer := time.AfterFunc(cfg.Timeout, func() {
		close(timedOut)
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			// Give 5s for graceful shutdown, then SIGKILL
			killTimer = time.AfterFunc(5*time.Second, func() {
				if cmd.Process != nil {
					syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
			})
		}
	})

	// Stream stdout and stderr concurrently
	done := make(chan struct{}, 2)

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			cfg.Encoder.Send(rpc.StdoutMessage{
				Type:   rpc.TypeStdout,
				Secret: cfg.Secret,
				Line:   scanner.Text(),
			})
		}
		done <- struct{}{}
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			cfg.Encoder.Send(rpc.StderrMessage{
				Type:   rpc.TypeStderr,
				Secret: cfg.Secret,
				Line:   scanner.Text(),
			})
		}
		done <- struct{}{}
	}()

	// Wait for both scanners to finish
	<-done
	<-done

	err = cmd.Wait()
	timer.Stop()
	if killTimer != nil {
		killTimer.Stop()
	}
	duration := time.Since(start)

	if err != nil {
		select {
		case <-timedOut:
			return ExecResult{ExitCode: -1, Duration: duration}
		default:
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return ExecResult{ExitCode: exitErr.ExitCode(), Duration: duration}
		}
		return ExecResult{ExitCode: -1, Duration: duration}
	}

	return ExecResult{ExitCode: 0, Duration: duration}
}
