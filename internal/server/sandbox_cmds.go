package server

import (
	"os"
	"path/filepath"

	"github.com/lableaks/fusebox/internal/sandbox"
)

// sandboxEnabled returns true — sandbox is always on for remote servers.
func sandboxEnabled() bool {
	return true
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

// ensureSandboxUp starts the sandbox if not already running. Returns an error
// instead of exiting the process, for use by internal callers like doCreate.
func ensureSandboxUp() error {
	sb := sandbox.New(defaultDataDir())
	st, err := sb.Status()
	if err == nil && st.Running {
		return nil // already running
	}
	cfg := sandbox.Config{
		BindMounts: defaultBindMounts(),
		Hostname:   "fusebox",
	}
	return sb.Up(cfg)
}
