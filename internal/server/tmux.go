package server

import (
	"os"
	"os/exec"
	"strings"
)

// tmuxSocket is set during init if a sandbox tmux socket is detected.
// When non-empty, all tmux commands use -S tmuxSocket.
var tmuxSocket string

func init() {
	// Auto-detect sandbox tmux socket. Single stat() call — negligible cost.
	sock := os.ExpandEnv("$HOME/.fusebox/tmux.sock")
	if info, err := os.Stat(sock); err == nil && !info.IsDir() {
		tmuxSocket = sock
	}
}

// tmuxArgs prepends -S socket to the args if a sandbox socket is set.
func tmuxArgs(args ...string) []string {
	if tmuxSocket != "" {
		return append([]string{"-S", tmuxSocket}, args...)
	}
	return args
}

// tmuxRun executes a tmux command and returns its stdout.
func tmuxRun(args ...string) (string, error) {
	cmd := exec.Command("tmux", tmuxArgs(args...)...)
	out, err := cmd.Output()
	return strings.TrimRight(string(out), "\n"), err
}

// tmuxHasSession checks if a specific tmux session exists.
func tmuxHasSession(name string) bool {
	return exec.Command("tmux", tmuxArgs("has-session", "-t", name)...).Run() == nil
}

// tmuxHasAnySession checks if any tmux sessions exist.
func tmuxHasAnySession() bool {
	return exec.Command("tmux", tmuxArgs("has-session")...).Run() == nil
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

// TmuxSocketPath returns the active tmux socket path (empty for default).
func TmuxSocketPath() string {
	return tmuxSocket
}
