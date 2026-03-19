package server

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadRoots reads browse roots from ~/.config/fusebox/roots.conf.
// Returns nil if the file doesn't exist.
func LoadRoots() ([]string, error) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "fusebox", "roots.conf")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseRoots(string(data)), nil
}

// parseRoots splits a roots.conf file into lines, skipping empties.
func parseRoots(content string) []string {
	var roots []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			roots = append(roots, line)
		}
	}
	return roots
}
