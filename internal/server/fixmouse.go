package server

// CmdFixMouse enables mouse mode on all existing tmux sessions.
func CmdFixMouse() {
	if !tmuxHasAnySession() {
		writeJSON(map[string]any{"ok": true, "fixed": 0})
		return
	}

	names, err := tmuxListSessions("#{session_name}")
	if err != nil {
		ExitError("list sessions: " + err.Error())
	}

	count := 0
	for _, name := range names {
		if _, err := tmuxRun("set-option", "-t", name, "mouse", "on"); err == nil {
			count++
		}
	}

	writeJSON(map[string]any{"ok": true, "fixed": count})
}
