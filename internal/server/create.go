package server

import (
	"fmt"
	"os"
	"strings"
)

const claudeFlags = "--dangerously-skip-permissions --remote-control"

// doCreate creates a new tmux session. Returns the expanded dir and any error.
func doCreate(name, dir string, teams bool) (string, error) {
	// Expand ~ if present
	if strings.HasPrefix(dir, "~") {
		home, _ := os.UserHomeDir()
		dir = home + dir[1:]
	}

	// Check directory exists
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return dir, fmt.Errorf("directory not found: %s", dir)
	}

	// Check session doesn't already exist
	if tmuxHasSession(name) {
		return dir, fmt.Errorf("session already exists: %s", name)
	}

	// Detect claude binary
	claudeBin, err := DetectClaude()
	if err != nil {
		return dir, err
	}

	// Create tmux session
	tmuxCmd := claudeBin + " " + claudeFlags
	args := []string{"new-session", "-d", "-s", name, "-c", dir, "-e", "WORK_SESSION=" + name}
	if teams {
		args = append(args, "-e", "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1")
	}
	args = append(args, tmuxCmd)
	if _, err := tmuxRun(args...); err != nil {
		return dir, fmt.Errorf("create session: %s", err.Error())
	}

	// Enable mouse mode
	tmuxRun("set-option", "-t", name, "mouse", "on")

	return dir, nil
}

// CmdCreate creates a new tmux session running claude in the given directory.
func CmdCreate(name, dir string) {
	expandedDir, err := doCreate(name, dir, false)
	if err != nil {
		ExitError(err.Error())
	}
	writeJSON(map[string]any{"ok": true, "name": name, "dir": expandedDir})
}

// CmdCreateTeam creates a new tmux session with agent teams enabled.
func CmdCreateTeam(name, dir string) {
	expandedDir, err := doCreate(name, dir, true)
	if err != nil {
		ExitError(err.Error())
	}
	writeJSON(map[string]any{"ok": true, "name": name, "dir": expandedDir, "teams": true})
}
