package server

import (
	"os/exec"
	"strings"
)

// tmuxRun executes a tmux command and returns its stdout.
func tmuxRun(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	return strings.TrimRight(string(out), "\n"), err
}

// tmuxHasSession checks if a specific tmux session exists.
func tmuxHasSession(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// tmuxHasAnySession checks if any tmux sessions exist.
func tmuxHasAnySession() bool {
	return exec.Command("tmux", "has-session").Run() == nil
}

// tmuxListSessions lists sessions with the given format string.
// Returns nil if no sessions exist.
func tmuxListSessions(format string) ([]string, error) {
	if !tmuxHasAnySession() {
		return nil, nil
	}
	out, err := tmuxRun("list-sessions", "-F", format)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}
