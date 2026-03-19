package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectClaude finds the claude binary by checking well-known paths, then PATH.
func DetectClaude() (string, error) {
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
