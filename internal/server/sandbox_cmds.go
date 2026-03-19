package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lableaks/fusebox/internal/sandbox"
)

// sandboxEnabled checks if sandbox mode is configured on the server.
func sandboxEnabled() bool {
	home, _ := os.UserHomeDir()
	// Check for the sandbox marker file
	_, err := os.Stat(filepath.Join(home, ".fusebox", "enabled"))
	return err == nil
}

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fusebox")
}

func syncedDirs() []sandbox.BindMount {
	syncBase := filepath.Join(defaultDataDir(), "sync")
	entries, err := os.ReadDir(syncBase)
	if err != nil {
		return nil
	}
	var mounts []sandbox.BindMount
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		hostPath := filepath.Join(syncBase, e.Name())
		mounts = append(mounts, sandbox.BindMount{
			Host:      hostPath,
			Container: hostPath,
		})
	}
	return mounts
}

func defaultBindMounts() []sandbox.BindMount {
	home, _ := os.UserHomeDir()
	mounts := syncedDirs()

	// Bind-mount .claude for settings, API key, session history
	claudeDir := filepath.Join(home, ".claude")
	if _, err := os.Stat(claudeDir); err == nil {
		mounts = append(mounts, sandbox.BindMount{
			Host:      claudeDir,
			Container: claudeDir,
		})
	}

	// Bind-mount activity hook directory
	activityDir := "/tmp/fusebox"
	os.MkdirAll(activityDir, 0755)
	mounts = append(mounts, sandbox.BindMount{
		Host:      activityDir,
		Container: activityDir,
	})

	return mounts
}

// CmdUp starts the sandbox.
func CmdUp() {
	sb := sandbox.New(defaultDataDir())

	cfg := sandbox.Config{
		BindMounts: defaultBindMounts(),
		Hostname:   "fusebox",
	}

	if err := sb.Up(cfg); err != nil {
		ExitError(err.Error())
	}

	writeJSON(map[string]any{
		"ok":     true,
		"socket": sb.TmuxSocket(),
	})
}

// CmdDown stops the sandbox.
func CmdDown() {
	sb := sandbox.New(defaultDataDir())
	if err := sb.Down(); err != nil {
		ExitError(err.Error())
	}
	writeJSON(map[string]any{"ok": true})
}

// CmdSandboxStatus reports the sandbox status.
func CmdSandboxStatus() {
	sb := sandbox.New(defaultDataDir())
	st, err := sb.Status()
	if err != nil {
		ExitError(err.Error())
	}

	// Also list synced directories
	syncBase := filepath.Join(defaultDataDir(), "sync")
	var synced []string
	if entries, err := os.ReadDir(syncBase); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				synced = append(synced, e.Name())
			}
		}
	}

	writeJSON(map[string]any{
		"running": st.Running,
		"pid":     st.PID,
		"socket":  st.Socket,
		"synced":  synced,
	})
}

// CmdUpdate updates Claude Code inside the sandbox.
func CmdUpdate() {
	sb := sandbox.New(defaultDataDir())
	st, err := sb.Status()
	if err != nil {
		ExitError(err.Error())
	}
	if !st.Running {
		ExitError("sandbox not running — run 'fusebox up' first")
	}

	out, err := sb.Exec([]string{"npm", "update", "-g", "@anthropic-ai/claude-code"})
	if err != nil {
		ExitError("update failed: " + strings.TrimSpace(string(out)))
	}

	writeJSON(map[string]any{"ok": true, "output": strings.TrimSpace(string(out))})
}
