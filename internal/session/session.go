package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lableaks/fusebox/internal/ssh"
)

type Session struct {
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Created  int64  `json:"created"`
	Activity int64  `json:"activity"`
}

// DisplayDir returns a shortened directory path for the table.
func (s Session) DisplayDir(homeDir string) string {
	dir := s.Dir
	if homeDir != "" {
		dir = strings.TrimPrefix(dir, homeDir+"/")
	}
	if len(dir) > 40 {
		dir = dir[:37] + "..."
	}
	return dir
}

// Uptime returns a human-readable uptime string.
func (s Session) Uptime() string {
	if s.Created == 0 {
		return "-"
	}
	d := time.Since(time.Unix(s.Created, 0))
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// Status returns "running" if recently active, otherwise "idle".
func (s Session) Status() string {
	if s.Activity == 0 {
		return "running"
	}
	idle := time.Since(time.Unix(s.Activity, 0))
	if idle < 30*time.Second {
		return "running"
	}
	return "idle"
}

// ToolActivity represents what a Claude session is currently doing.
type ToolActivity struct {
	Tool   string `json:"tool"`
	Detail string `json:"detail"`
	TS     int64  `json:"ts"`
}

// DisplayStatus returns a human-readable status string for the dashboard.
func (a ToolActivity) DisplayStatus() string {
	if a.Detail == "" {
		return a.Tool
	}
	switch a.Tool {
	case "Bash":
		return "Bash: " + a.Detail
	case "Edit", "Read", "Write":
		return a.Tool + " " + a.Detail
	default:
		return a.Tool + ": " + a.Detail
	}
}

// Manager handles session CRUD operations via SSH.
type Manager struct {
	SSH        ssh.Runner
	serverPath string
}

func NewManager(runner ssh.Runner, serverPath string) *Manager {
	return &Manager{SSH: runner, serverPath: serverPath}
}

func (m *Manager) List() ([]Session, error) {
	out, err := m.SSH.Run(m.serverPath + " list")
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var sessions []Session
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, fmt.Errorf("parse sessions: %w (output: %s)", err, string(out))
	}
	return sessions, nil
}

func (m *Manager) Create(name, dir string) error {
	cmd := fmt.Sprintf(m.serverPath + " create %s %s", name, dir)
	if _, err := m.SSH.Run(cmd); err != nil {
		return fmt.Errorf("create session %q: %w", name, err)
	}
	return nil
}

func (m *Manager) Stop(name string) error {
	cmd := fmt.Sprintf(m.serverPath + " stop %s", name)
	if _, err := m.SSH.Run(cmd); err != nil {
		return fmt.Errorf("stop session %q: %w", name, err)
	}
	return nil
}

func (m *Manager) Preview(name string, lines int) (string, error) {
	cmd := fmt.Sprintf(m.serverPath + " preview %s %d", name, lines)
	out, err := m.SSH.Run(cmd)
	if err != nil {
		return "", fmt.Errorf("preview %q: %w", name, err)
	}
	return string(out), nil
}

func (m *Manager) FetchActivity() (map[string]ToolActivity, error) {
	out, err := m.SSH.Run(m.serverPath + " activity")
	if err != nil {
		return nil, fmt.Errorf("fetch activity: %w", err)
	}

	var activity map[string]ToolActivity
	if err := json.Unmarshal(out, &activity); err != nil {
		return nil, fmt.Errorf("parse activity: %w (output: %s)", err, string(out))
	}
	return activity, nil
}

func (m *Manager) ListDirs() ([]string, error) {
	out, err := m.SSH.Run(m.serverPath + " dirs")
	if err != nil {
		return nil, fmt.Errorf("list dirs: %w", err)
	}

	var dirs []string
	if err := json.Unmarshal(out, &dirs); err != nil {
		return nil, fmt.Errorf("parse dirs: %w (output: %s)", err, string(out))
	}
	return dirs, nil
}

// SubdirEntry matches server.DirEntry JSON.
type SubdirEntry struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

func (m *Manager) ListSubdirs(path string) ([]SubdirEntry, error) {
	out, err := m.SSH.Run(fmt.Sprintf("%s subdirs %s", m.serverPath, path))
	if err != nil {
		return nil, fmt.Errorf("list subdirs: %w", err)
	}

	var entries []SubdirEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse subdirs: %w (output: %s)", err, string(out))
	}
	return entries, nil
}

// Team-related types.

type TeamMember struct {
	Name      string `json:"name"`
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

type TeamTask struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	State      string `json:"state"`
	AssignedTo string `json:"assigned_to,omitempty"`
}

type TeamStatus struct {
	Name       string       `json:"name"`
	Members    []TeamMember `json:"members"`
	Tasks      []TeamTask   `json:"tasks"`
	Pending    int          `json:"pending"`
	InProgress int          `json:"in_progress"`
	Completed  int          `json:"completed"`
	Total      int          `json:"total"`
}

type PaneInfo struct {
	Index   int    `json:"index"`
	Title   string `json:"title"`
	Command string `json:"command"`
	Active  bool   `json:"active"`
}

func (m *Manager) ListTeams() ([]TeamStatus, error) {
	out, err := m.SSH.Run(m.serverPath + " teams")
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}

	var teams []TeamStatus
	if err := json.Unmarshal(out, &teams); err != nil {
		return nil, fmt.Errorf("parse teams: %w (output: %s)", err, string(out))
	}
	return teams, nil
}

func (m *Manager) TeamsToggle(enable bool) error {
	onOff := "off"
	if enable {
		onOff = "on"
	}
	cmd := fmt.Sprintf("%s teams-toggle %s", m.serverPath, onOff)
	if _, err := m.SSH.Run(cmd); err != nil {
		return fmt.Errorf("teams toggle: %w", err)
	}
	return nil
}

func (m *Manager) ListPanes(session string) ([]PaneInfo, error) {
	cmd := fmt.Sprintf("%s panes %s", m.serverPath, session)
	out, err := m.SSH.Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("list panes: %w", err)
	}

	var panes []PaneInfo
	if err := json.Unmarshal(out, &panes); err != nil {
		return nil, fmt.Errorf("parse panes: %w (output: %s)", err, string(out))
	}
	return panes, nil
}

func (m *Manager) PanePreview(session string, pane int, lines int) (string, error) {
	cmd := fmt.Sprintf("%s pane-preview %s %d %d", m.serverPath, session, pane, lines)
	out, err := m.SSH.Run(cmd)
	if err != nil {
		return "", fmt.Errorf("pane preview: %w", err)
	}
	return string(out), nil
}

func (m *Manager) CreateTeam(name, dir string) error {
	cmd := fmt.Sprintf("%s create-team %s %s", m.serverPath, name, dir)
	if _, err := m.SSH.Run(cmd); err != nil {
		return fmt.Errorf("create team session %q: %w", name, err)
	}
	return nil
}
