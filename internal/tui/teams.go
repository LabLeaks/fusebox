package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"

	"github.com/lableaks/fusebox/internal/session"
)

type teamDetailModel struct {
	status      session.TeamStatus
	panes       []session.PaneInfo
	cursor      int
	preview     viewport.Model
	showPreview bool
	session     string // tmux session name
}

func newTeamDetail(status session.TeamStatus, panes []session.PaneInfo, sessionName string) teamDetailModel {
	return teamDetailModel{
		status:  status,
		panes:   panes,
		preview: viewport.New(),
		session: sessionName,
	}
}

func (m teamDetailModel) memberCount() int {
	// Use panes if available (more accurate), else members from config
	if len(m.panes) > 0 {
		return len(m.panes)
	}
	return len(m.status.Members)
}

func (m teamDetailModel) Update(msg tea.Msg) (teamDetailModel, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		count := m.memberCount()
		switch kmsg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < count-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m teamDetailModel) View() string {
	var b strings.Builder

	header := fmt.Sprintf("  Team: %s  ·  %s  ", m.status.Name, m.session)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n\n")

	// Teammates section
	b.WriteString("  Teammates\n")

	if len(m.panes) > 0 {
		for i, p := range m.panes {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			name := p.Title
			if name == "" {
				if i == 0 {
					name = "lead"
				} else {
					name = fmt.Sprintf("pane-%d", p.Index)
				}
			}

			detail := p.Command
			style := teamMemberIdle
			if p.Active {
				style = teamMemberActive
			}

			line := fmt.Sprintf("  %s%-18s %s", cursor, name, style.Render(detail))
			b.WriteString(line)
			b.WriteString("\n")
		}
	} else if len(m.status.Members) > 0 {
		for i, member := range m.status.Members {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			name := member.Name
			agentType := member.AgentType
			if agentType == "" {
				agentType = "agent"
			}

			line := fmt.Sprintf("  %s%-18s %s", cursor, name, teamMemberIdle.Render(agentType))
			b.WriteString(line)
			b.WriteString("\n")
		}
	} else {
		b.WriteString("    No teammates detected\n")
	}

	// Tasks section
	if m.status.Total > 0 {
		b.WriteString(fmt.Sprintf("\n  Tasks (%d/%d)\n", m.status.Completed, m.status.Total))
		for _, task := range m.status.Tasks {
			var icon, line string
			switch task.State {
			case "completed":
				icon = taskDone.Render("✓")
				line = fmt.Sprintf("    %s %s", icon, task.Title)
			case "in_progress":
				icon = taskInProgress.Render("●")
				assigned := ""
				if task.AssignedTo != "" {
					assigned = "  " + teamMemberIdle.Render(task.AssignedTo)
				}
				line = fmt.Sprintf("    %s %s%s", icon, task.Title, assigned)
			default:
				line = fmt.Sprintf("      %s", task.Title)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// Preview
	if m.showPreview {
		b.WriteString("\n")
		b.WriteString(m.preview.View())
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  [enter] attach  [p] preview  [esc] back"))

	return b.String()
}
