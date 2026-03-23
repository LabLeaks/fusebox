package server

import (
	"fmt"
	"os"
	"strings"
)

const claudeFlags = "--dangerously-skip-permissions --remote-control"

type createOpts struct {
	Teams  bool
	Resume bool
}

// doCreate creates a new tmux session. Returns the expanded dir and any error.
func doCreate(name, dir string, opts createOpts) (string, error) {
	// Expand ~ if present
	if strings.HasPrefix(dir, "~") {
		home, _ := os.UserHomeDir()
		dir = home + dir[1:]
	}

	// Auto-start sandbox if not already running
	if sandboxEnabled() && tmuxSocket == "" {
		if err := ensureSandboxUp(); err != nil {
			return dir, fmt.Errorf("sandbox: %w", err)
		}
		// Re-detect socket after starting
		sock := os.ExpandEnv("$HOME/.fusebox/tmux.sock")
		if info, err := os.Stat(sock); err == nil && !info.IsDir() {
			tmuxSocket = sock
		}
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
	// Apply defaults from server config
	if defaults := LoadDefaults(); defaults != nil {
		if defaults["model"] != "" {
			tmuxCmd += " --model " + defaults["model"]
		}
		if defaults["effort"] != "" {
			tmuxCmd += " --effort " + defaults["effort"]
		}
	}
	if opts.Resume {
		tmuxCmd += " --resume"
	}
	args := []string{"new-session", "-d", "-s", name, "-c", dir, "-e", "FUSEBOX_SESSION=" + name}
	if opts.Teams {
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
	expandedDir, err := doCreate(name, dir, createOpts{})
	if err != nil {
		ExitError(err.Error())
	}
	writeJSON(map[string]any{"ok": true, "name": name, "dir": expandedDir})
}

// CmdCreateTeam creates a new tmux session with agent teams enabled.
func CmdCreateTeam(name, dir string) {
	expandedDir, err := doCreate(name, dir, createOpts{Teams: true})
	if err != nil {
		ExitError(err.Error())
	}
	writeJSON(map[string]any{"ok": true, "name": name, "dir": expandedDir, "teams": true})
}

// CmdCreateResume creates a new tmux session that resumes the last conversation.
func CmdCreateResume(name, dir string) {
	expandedDir, err := doCreate(name, dir, createOpts{Resume: true})
	if err != nil {
		ExitError(err.Error())
	}
	writeJSON(map[string]any{"ok": true, "name": name, "dir": expandedDir, "resume": true})
}

// CmdCreateTeamResume creates a team session that resumes the last conversation.
func CmdCreateTeamResume(name, dir string) {
	expandedDir, err := doCreate(name, dir, createOpts{Teams: true, Resume: true})
	if err != nil {
		ExitError(err.Error())
	}
	writeJSON(map[string]any{"ok": true, "name": name, "dir": expandedDir, "teams": true, "resume": true})
}
