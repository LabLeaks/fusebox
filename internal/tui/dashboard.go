package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"

	"github.com/lableaks/fusebox/internal/session"
)

type dashboardModel struct {
	sessions     []session.Session
	activity     map[string]session.ToolActivity
	table        table.Model
	err          error
	width        int
	height       int
	confirm      string          // non-empty = confirming stop for this session name
	pending      map[string]bool // sessions being created
	stopping     map[string]bool // sessions being stopped
	serverHost   string
	homeDir      string
	teamSessions map[string]string    // team name → session name
	teams        []session.TeamStatus // all teams
}

func newDashboard(sessions []session.Session, serverHost, homeDir string) dashboardModel {
	cols := []table.Column{
		{Title: "Session", Width: 20},
		{Title: "Directory", Width: 40},
		{Title: "Uptime", Width: 10},
		{Title: "Status", Width: 30},
	}

	rows := sessionsToRows(sessions, nil, nil, nil, homeDir, nil, nil)

	tableWidth := 0
	for _, c := range cols {
		tableWidth += c.Width + 2 // column width + cell padding
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithWidth(tableWidth),
		table.WithHeight(10),
		table.WithFocused(true),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#5A3DB5")).
		Bold(true)
	t.SetStyles(s)

	return dashboardModel{
		sessions:   sessions,
		table:      t,
		serverHost: serverHost,
		homeDir:    homeDir,
	}
}

func sessionsToRows(sessions []session.Session, activity map[string]session.ToolActivity, pending, stopping map[string]bool, homeDir string, teamSessions map[string]string, teams []session.TeamStatus) []table.Row {
	rows := make([]table.Row, len(sessions))
	for i, s := range sessions {
		rows[i] = table.Row{s.Name, s.DisplayDir(homeDir), s.Uptime(), formatStatus(s, activity, pending, stopping, teamSessions, teams)}
	}
	return rows
}

func formatStatus(s session.Session, activity map[string]session.ToolActivity, pending, stopping map[string]bool, teamSessions map[string]string, teams []session.TeamStatus) string {
	if pending[s.Name] {
		return statusCreating.String()
	}
	if stopping[s.Name] {
		return statusStopping.String()
	}
	// Check if this session is a team lead
	for teamName, sessionName := range teamSessions {
		if sessionName == s.Name {
			for _, ts := range teams {
				if ts.Name == teamName {
					return statusTeam.Render(fmt.Sprintf("Team: %s %d/%d", ts.Name, ts.Completed, ts.Total))
				}
			}
		}
	}
	if a, ok := activity[s.Name]; ok {
		return statusActive.Render(a.DisplayStatus())
	}
	if s.Status() == "running" {
		return statusRunning.String()
	}
	return statusIdle.String()
}

func (m dashboardModel) selectedSession() (session.Session, bool) {
	if len(m.sessions) == 0 {
		return session.Session{}, false
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.sessions) {
		return session.Session{}, false
	}
	return m.sessions[idx], true
}

func (m *dashboardModel) removeSession(name string) {
	if m.stopping == nil {
		m.stopping = make(map[string]bool)
	}
	m.stopping[name] = true
	m.refreshRows()
}

func (m *dashboardModel) addPendingSession(name, dir string) {
	s := session.Session{
		Name:     name,
		Dir:      dir,
		Created:  time.Now().Unix(),
		Activity: time.Now().Unix(),
	}
	m.sessions = append([]session.Session{s}, m.sessions...)
	if m.pending == nil {
		m.pending = make(map[string]bool)
	}
	m.pending[name] = true
	m.refreshRows()
	m.table.SetCursor(0)
}

func (m *dashboardModel) updateActivity(activity map[string]session.ToolActivity) {
	m.activity = activity
	m.refreshRows()
}

func (m *dashboardModel) updateTeams(teamSessions map[string]string, teams []session.TeamStatus) {
	m.teamSessions = teamSessions
	m.teams = teams
	m.refreshRows()
}

func (m *dashboardModel) refreshRows() {
	m.table.SetRows(sessionsToRows(m.sessions, m.activity, m.pending, m.stopping, m.homeDir, m.teamSessions, m.teams))
}

func (m dashboardModel) Update(msg tea.Msg) (dashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// If confirming a stop, handle y/n
		if m.confirm != "" {
			switch msg.String() {
			case "y":
				name := m.confirm
				m.confirm = ""
				return m, stopSessionCmd(name)
			default:
				m.confirm = ""
				return m, nil
			}
		}

		switch msg.String() {
		case keyStop:
			if s, ok := m.selectedSession(); ok {
				m.confirm = s.Name
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m dashboardModel) View() string {
	var b strings.Builder

	count := len(m.sessions)
	header := fmt.Sprintf("  FUSEBOX  ·  %s  ·  %d active session", m.serverHost, count)
	if count != 1 {
		header += "s"
	}
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	if len(m.sessions) == 0 {
		b.WriteString("  No active sessions. Press [n] to create one.\n\n")
	} else {
		b.WriteString(m.table.View())
		b.WriteString("\n\n")
	}

	if m.confirm != "" {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Stop session %q? [y/n]", m.confirm)))
		b.WriteString("\n\n")
	}

	help := "  [n] new session  [enter] attach"
	// Show [t] team if selected session is a team lead
	if s, ok := m.selectedSession(); ok {
		for _, sName := range m.teamSessions {
			if sName == s.Name {
				help += "  [t] team"
				break
			}
		}
	}
	help += "  [d] stop  [s] synced folders  [p] preview  [q] quit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}
