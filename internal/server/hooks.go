package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// updateHooksSettings processes a settings.json blob to install a PostToolUse hook.
// Returns the updated JSON and a status message.
// If the hook is already installed, returns the original data unchanged.
func updateHooksSettings(settingsData []byte, hookCmd string) ([]byte, string, error) {
	var settings map[string]any
	if len(settingsData) == 0 {
		settings = map[string]any{}
	} else {
		if err := json.Unmarshal(settingsData, &settings); err != nil {
			return nil, "", err
		}
	}

	// Navigate to hooks.PostToolUse
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}

	postToolUseRaw, _ := hooks["PostToolUse"].([]any)

	// Check if hook already installed
	for _, entry := range postToolUseRaw {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := m["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range hooksList {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hm["command"].(string); ok && cmd == hookCmd {
				return settingsData, "hook already installed", nil
			}
		}
	}

	// Remove old-format entries (without "hooks" key) and old work-hook entries
	var filtered []any
	for _, entry := range postToolUseRaw {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooksList, ok := m["hooks"].([]any)
		if !ok {
			continue // old format, skip
		}
		// Skip entries pointing to old work-hook binary
		isOld := false
		for _, h := range hooksList {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hm["command"].(string)
			if strings.HasSuffix(cmd, "/work-hook") {
				isOld = true
				break
			}
		}
		if !isOld {
			filtered = append(filtered, entry)
		}
	}

	// Add new hook entry
	newEntry := map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hookCmd,
			},
		},
	}
	filtered = append(filtered, newEntry)
	hooks["PostToolUse"] = filtered

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, "", err
	}
	out = append(out, '\n')
	return out, "hook installed", nil
}

// CmdInstallHooks installs the Claude Code PostToolUse hook in settings.json.
func CmdInstallHooks() {
	home, _ := os.UserHomeDir()
	settingsFile := filepath.Join(home, ".claude", "settings.json")

	// Determine hook command: this binary + "hook"
	exe, err := os.Executable()
	if err != nil {
		ExitError("cannot determine executable path: " + err.Error())
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		ExitError("cannot resolve executable path: " + err.Error())
	}
	hookCmd := exe + " hook"

	// Ensure .claude directory exists
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)

	// Read existing settings
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		if !os.IsNotExist(err) {
			ExitError("read settings: " + err.Error())
		}
		data = nil
	}

	out, msg, err := updateHooksSettings(data, hookCmd)
	if err != nil {
		ExitError(err.Error())
	}

	// Only write if we actually changed something
	if msg != "hook already installed" {
		if err := os.WriteFile(settingsFile, out, 0644); err != nil {
			ExitError("write settings: " + err.Error())
		}
	}

	writeJSON(map[string]any{"ok": true, "message": msg})
}
