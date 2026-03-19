package server

import "fmt"

// doStop kills a tmux session. Returns an error instead of exiting.
func doStop(name string) error {
	if !tmuxHasSession(name) {
		return fmt.Errorf("session not found: %s", name)
	}
	if _, err := tmuxRun("kill-session", "-t", name); err != nil {
		return fmt.Errorf("stop session: %s", err.Error())
	}
	return nil
}

// CmdStop kills a tmux session by name.
func CmdStop(name string) {
	if err := doStop(name); err != nil {
		ExitError(err.Error())
	}
	writeJSON(map[string]any{"ok": true})
}
