package orchestrator

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/lableaks/fusebox/internal/sync"
)

// ContainerManager is the subset of container.Manager used by Down.
type ContainerManager interface {
	Stop(projectName string) error
	Remove(projectName string) error
}

// DownOptions controls the behavior of fusebox down.
type DownOptions struct {
	ProjectName string
	Destroy     bool
	Force       bool
}

// daemonStatus represents the JSON structure from the status socket.
type daemonStatus struct {
	LastAction    string `json:"last_action"`
	ActionRunning bool   `json:"action_running"`
}

// Down stops a fusebox session gracefully.
func Down(opts DownOptions, mutagen *sync.MutagenManager, container ContainerManager, output func(string)) error {
	if output == nil {
		output = func(s string) { fmt.Println(s) }
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	runDir := filepath.Join(homeDir, ".fusebox", "run")

	// Check if an action is running (unless --force)
	if !opts.Force {
		running, action := checkActionRunning(runDir, opts.ProjectName)
		if running {
			return fmt.Errorf("action %q is currently running — use --force to stop anyway", action)
		}
	}

	// Stop local daemon via PID file
	stopDaemon(runDir, opts.ProjectName, output)

	srcSession := sync.SrcSessionName(opts.ProjectName)
	claudeSession := sync.ClaudeSessionName(opts.ProjectName)

	if opts.Destroy {
		// Terminate Mutagen sessions (removes cache)
		terminateSession(mutagen, srcSession, output)
		terminateSession(mutagen, claudeSession, output)

		// Stop and remove container
		if err := container.Stop(opts.ProjectName); err != nil {
			if !isAlreadyStopped(err) {
				output(fmt.Sprintf("Warning: stopping container: %s", err))
			}
		}
		if err := container.Remove(opts.ProjectName); err != nil {
			if !isNotFound(err) {
				output(fmt.Sprintf("Warning: removing container: %s", err))
			}
		}

		output("Destroyed.")
	} else {
		// Pause Mutagen sessions (preserves cache for warm restart)
		pauseSession(mutagen, srcSession, output)
		pauseSession(mutagen, claudeSession, output)

		output("Container left running (warm restart available).")
	}

	return nil
}

// stopDaemon reads the PID file and sends SIGTERM to the daemon process.
func stopDaemon(runDir, projectName string, output func(string)) {
	pidFile := filepath.Join(runDir, projectName+".pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			output("Daemon not running (no PID file).")
			return
		}
		output(fmt.Sprintf("Warning: reading PID file: %s", err))
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		output(fmt.Sprintf("Warning: invalid PID file contents: %s", err))
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		output(fmt.Sprintf("Warning: finding process %d: %s", pid, err))
		return
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if isProcessGone(err) {
			output("Daemon already stopped.")
		} else {
			output(fmt.Sprintf("Warning: sending SIGTERM to daemon: %s", err))
		}
	} else {
		output("Daemon stopped.")
	}

	// Clean up PID file
	os.Remove(pidFile)
}

// checkActionRunning reads the status socket to see if an action is in progress.
func checkActionRunning(runDir, projectName string) (bool, string) {
	sockPath := filepath.Join(runDir, projectName+".sock")
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return false, ""
	}
	defer conn.Close()

	var status daemonStatus
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&status); err != nil {
		return false, ""
	}

	return status.ActionRunning, status.LastAction
}

// pauseSession pauses a Mutagen session, skipping gracefully if already paused or not found.
func pauseSession(mutagen *sync.MutagenManager, sessionName string, output func(string)) {
	if err := mutagen.Pause(sessionName); err != nil {
		if isSessionNotFound(err) || isAlreadyPaused(err) {
			return
		}
		output(fmt.Sprintf("Warning: pausing %s: %s", sessionName, err))
	}
}

// terminateSession terminates a Mutagen session, skipping gracefully if not found.
func terminateSession(mutagen *sync.MutagenManager, sessionName string, output func(string)) {
	if err := mutagen.Terminate(sessionName); err != nil {
		if isSessionNotFound(err) {
			return
		}
		output(fmt.Sprintf("Warning: terminating %s: %s", sessionName, err))
	}
}

func isProcessGone(err error) bool {
	return strings.Contains(err.Error(), "process already finished") ||
		strings.Contains(err.Error(), "no such process")
}

func isSessionNotFound(err error) bool {
	return strings.Contains(err.Error(), "unable to locate") ||
		strings.Contains(err.Error(), "session not found")
}

func isAlreadyPaused(err error) bool {
	return strings.Contains(err.Error(), "already paused")
}

func isAlreadyStopped(err error) bool {
	return strings.Contains(err.Error(), "is not running") ||
		strings.Contains(err.Error(), "No such container")
}

func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "No such container") ||
		strings.Contains(err.Error(), "not found")
}
