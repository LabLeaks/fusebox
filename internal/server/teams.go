package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Types for team-related data.

type paneInfo struct {
	Index   int    `json:"index"`
	Title   string `json:"title"`
	Command string `json:"command"`
	Active  bool   `json:"active"`
}

type teamMember struct {
	Name      string `json:"name"`
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

type teamTask struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	State      string `json:"state"` // "pending", "in_progress", "completed"
	AssignedTo string `json:"assigned_to,omitempty"`
}

type teamStatus struct {
	Name       string       `json:"name"`
	Members    []teamMember `json:"members"`
	Tasks      []teamTask   `json:"tasks"`
	Pending    int          `json:"pending"`
	InProgress int          `json:"in_progress"`
	Completed  int          `json:"completed"`
	Total      int          `json:"total"`
}

// CmdTeams reads team configs and tasks from ~/.claude/teams/ and ~/.claude/tasks/.
func CmdTeams() {
	home, _ := os.UserHomeDir()
	teams := loadTeams(home)
	if teams == nil {
		teams = []teamStatus{}
	}
	writeJSON(teams)
}

func loadTeams(home string) []teamStatus {
	teamsDir := filepath.Join(home, ".claude", "teams")
	tasksDir := filepath.Join(home, ".claude", "tasks")

	entries, err := os.ReadDir(teamsDir)
	if err != nil {
		return nil
	}

	var teams []teamStatus
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		teamName := e.Name()
		ts := teamStatus{Name: teamName}

		// Read config
		configPath := filepath.Join(teamsDir, teamName, "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var config struct {
				Members []teamMember `json:"members"`
			}
			if json.Unmarshal(data, &config) == nil {
				ts.Members = config.Members
			}
		}

		// Read tasks
		taskDir := filepath.Join(tasksDir, teamName)
		if taskEntries, err := os.ReadDir(taskDir); err == nil {
			for _, te := range taskEntries {
				if te.IsDir() || !strings.HasSuffix(te.Name(), ".json") {
					continue
				}
				taskData, err := os.ReadFile(filepath.Join(taskDir, te.Name()))
				if err != nil {
					continue
				}
				var task teamTask
				if json.Unmarshal(taskData, &task) != nil {
					continue
				}
				if task.ID == "" {
					task.ID = strings.TrimSuffix(te.Name(), ".json")
				}
				ts.Tasks = append(ts.Tasks, task)
				switch task.State {
				case "pending":
					ts.Pending++
				case "in_progress":
					ts.InProgress++
				case "completed":
					ts.Completed++
				}
			}
		}
		ts.Total = len(ts.Tasks)
		teams = append(teams, ts)
	}

	return teams
}

// CmdTeamsToggle enables or disables agent teams in settings.json.
func CmdTeamsToggle(onOff string) {
	enable := onOff == "on"

	home, _ := os.UserHomeDir()
	settingsFile := filepath.Join(home, ".claude", "settings.json")

	os.MkdirAll(filepath.Join(home, ".claude"), 0755)

	data, err := os.ReadFile(settingsFile)
	if err != nil {
		if !os.IsNotExist(err) {
			ExitError("read settings: " + err.Error())
		}
		data = nil
	}

	out, msg, err := updateTeamsSettings(data, enable)
	if err != nil {
		ExitError(err.Error())
	}

	if msg != "no change" {
		if err := os.WriteFile(settingsFile, out, 0644); err != nil {
			ExitError("write settings: " + err.Error())
		}
	}

	writeJSON(map[string]any{"ok": true, "message": msg})
}

// updateTeamsSettings is a pure function that enables/disables CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS
// in the env block of settings.json.
func updateTeamsSettings(data []byte, enable bool) ([]byte, string, error) {
	var settings map[string]any
	if len(data) == 0 {
		settings = map[string]any{}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, "", err
		}
	}

	env, _ := settings["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}

	const key = "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"

	if enable {
		if env[key] == "1" {
			return data, "no change", nil
		}
		env[key] = "1"
		settings["env"] = env
	} else {
		if _, exists := env[key]; !exists {
			return data, "no change", nil
		}
		delete(env, key)
		if len(env) == 0 {
			delete(settings, "env")
		} else {
			settings["env"] = env
		}
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, "", err
	}
	out = append(out, '\n')

	if enable {
		return out, "teams enabled", nil
	}
	return out, "teams disabled", nil
}

// CmdPanes lists tmux panes for a session.
func CmdPanes(session string) {
	if !tmuxHasSession(session) {
		ExitError("session not found: " + session)
	}

	out, err := tmuxRun("list-panes", "-t", session,
		"-F", "#{pane_index}|#{pane_title}|#{pane_current_command}|#{pane_active}")
	if err != nil {
		ExitError("list panes: " + err.Error())
	}

	panes := []paneInfo{}
	if out == "" {
		writeJSON(panes)
		return
	}
	for _, line := range strings.Split(out, "\n") {
		p, err := parsePaneLine(line)
		if err != nil {
			continue
		}
		panes = append(panes, p)
	}

	writeJSON(panes)
}

func parsePaneLine(line string) (paneInfo, error) {
	parts := strings.SplitN(line, "|", 4)
	if len(parts) < 4 {
		return paneInfo{}, fmt.Errorf("invalid pane line: %s", line)
	}

	idx := 0
	fmt.Sscanf(parts[0], "%d", &idx)

	return paneInfo{
		Index:   idx,
		Title:   parts[1],
		Command: parts[2],
		Active:  parts[3] == "1",
	}, nil
}

// CmdPanePreview captures a specific pane's content.
func CmdPanePreview(session, paneIndex, lines string) {
	if !tmuxHasSession(session) {
		ExitError("session not found: " + session)
	}

	target := fmt.Sprintf("%s:0.%s", session, paneIndex)
	out, err := tmuxRun("capture-pane", "-t", target, "-p", "-S", fmt.Sprintf("-%s", lines))
	if err != nil {
		ExitError("capture pane: " + err.Error())
	}

	fmt.Println(out)
}
