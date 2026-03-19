package server

import (
	"fmt"
	"strconv"
	"strings"
)

type sessionInfo struct {
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Created  int64  `json:"created"`
	Activity int64  `json:"activity"`
}

// parseSessionLine parses a tmux list-sessions line in the format:
// name|path|created|activity
func parseSessionLine(line string) (sessionInfo, error) {
	parts := strings.SplitN(line, "|", 4)
	if len(parts) != 4 {
		return sessionInfo{}, fmt.Errorf("invalid session line: %s", line)
	}
	created, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return sessionInfo{}, fmt.Errorf("invalid created timestamp: %s", parts[2])
	}
	activity, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return sessionInfo{}, fmt.Errorf("invalid activity timestamp: %s", parts[3])
	}
	return sessionInfo{
		Name:     parts[0],
		Dir:      parts[1],
		Created:  created,
		Activity: activity,
	}, nil
}

// getSessions returns all tmux sessions.
func getSessions() ([]sessionInfo, error) {
	lines, err := tmuxListSessions("#{session_name}|#{session_path}|#{session_created}|#{session_activity}")
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	if lines == nil {
		return []sessionInfo{}, nil
	}
	sessions := make([]sessionInfo, 0, len(lines))
	for _, line := range lines {
		s, err := parseSessionLine(line)
		if err != nil {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// CmdList outputs all tmux sessions as a JSON array.
func CmdList() {
	sessions, err := getSessions()
	if err != nil {
		ExitError(err.Error())
	}
	writeJSON(sessions)
}
