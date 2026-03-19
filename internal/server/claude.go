package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectClaude finds the claude binary by checking well-known paths, then PATH.
// If running inside a sandbox (tmuxSocket is set), uses the known rootfs path.
func DetectClaude() (string, error) {
	// Inside a sandbox, claude is at a known path in the rootfs
	if tmuxSocket != "" {
		sandboxPath := "/usr/local/bin/claude"
		if _, err := os.Stat(sandboxPath); err == nil {
			return sandboxPath, nil
		}
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "claude"),
		"/usr/local/bin/claude",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("claude not found: checked %s and PATH", strings.Join(candidates, ", "))
}
