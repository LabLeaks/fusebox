package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// getDirs returns browsable directories from roots.conf.
func getDirs() ([]string, error) {
	roots, err := LoadRoots()
	if err != nil {
		return nil, fmt.Errorf("load roots: %w", err)
	}
	var dirs []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				dirs = append(dirs, filepath.Join(root, entry.Name()))
			}
		}
	}
	if dirs == nil {
		dirs = []string{}
	}
	return dirs, nil
}

// CmdDirs outputs browsable directories as a JSON array.
func CmdDirs() {
	dirs, err := getDirs()
	if err != nil {
		ExitError(err.Error())
	}
	writeJSON(dirs)
}

// DirEntry is a directory with its subdirectory count.
type DirEntry struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// CmdSubdirs lists subdirectories of the given path with subdir counts.
func CmdSubdirs(path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		ExitError(fmt.Sprintf("read dir: %v", err))
	}
	var result []DirEntry
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		sub := filepath.Join(path, entry.Name())
		count := 0
		if subEntries, err := os.ReadDir(sub); err == nil {
			for _, se := range subEntries {
				if se.IsDir() {
					count++
				}
			}
		}
		result = append(result, DirEntry{Path: entry.Name(), Count: count})
	}
	if result == nil {
		result = []DirEntry{}
	}
	writeJSON(result)
}
