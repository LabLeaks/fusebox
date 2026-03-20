package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/lableaks/fusebox/internal/config"
	syncpkg "github.com/lableaks/fusebox/internal/sync"
)

// Messages for settings view.

type syncSessionsLoadedMsg struct {
	sessions []syncpkg.SyncSession
	err      error
}

type syncAddedMsg struct {
	path string
	err  error
}

type syncRemovedMsg struct {
	name string
	err  error
}

type localDirsLoadedMsg struct {
	entries []dirBrowserEntry
	err     error
}

type defaultsSavedMsg struct {
	err error
}

var models = []string{"sonnet", "opus", "haiku"}
var efforts = []string{"low", "medium", "high", "max"}

// settingsModel manages the settings view.
type settingsModel struct {
	// Synced folders
	sessions    []syncpkg.SyncSession
	syncCursor  int
	syncConfirm string
	syncErr     error
	adding      bool
	browser     dirBrowser
	mgr         *syncpkg.Manager
}

func newSettingsModel(mgr *syncpkg.Manager) settingsModel {
	home, _ := os.UserHomeDir()
	return settingsModel{
		mgr:     mgr,
		browser: newDirBrowser(home),
	}
}

func (m settingsModel) Update(msg tea.Msg) (settingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case syncSessionsLoadedMsg:
		if msg.err != nil {
			m.syncErr = msg.err
		} else {
			m.sessions = msg.sessions
			m.syncErr = nil
		}
		if m.syncCursor >= len(m.sessions) {
			m.syncCursor = max(0, len(m.sessions)-1)
		}
		return m, nil

	case syncAddedMsg:
		m.adding = false
		if msg.err != nil {
			m.syncErr = msg.err
		} else {
			m.syncErr = nil
		}
		return m, loadSyncSessionsCmd(m.mgr)

	case syncRemovedMsg:
		m.syncConfirm = ""
		if msg.err != nil {
			m.syncErr = msg.err
		} else {
			m.syncErr = nil
		}
		return m, loadSyncSessionsCmd(m.mgr)

	case localDirsLoadedMsg:
		if msg.err != nil {
			m.browser.SetError(msg.err)
		} else {
			m.browser.SetEntries(msg.entries)
		}
		return m, nil

	case defaultsSavedMsg:
		return m, nil
	}

	return m, nil
}

func (m settingsModel) View(cfg config.Config) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  Settings  "))
	b.WriteString("\n\n")

	if m.adding {
		return m.viewSyncAdd(&b)
	}

	// Synced Folders
	b.WriteString("  Synced Folders\n")
	b.WriteString("  " + helpStyle.Render("Files sync both ways — your IDE and Claude see the same code.") + "\n\n")

	if m.syncErr != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %v", m.syncErr)))
		b.WriteString("\n\n")
	}

	if len(m.sessions) == 0 {
		b.WriteString("  " + helpStyle.Render("No folders synced yet.") + "\n\n")
	} else {
		for i, s := range m.sessions {
			cursor := "  "
			if i == m.syncCursor {
				cursor = "▸ "
			}
			local := s.Local
			if home, err := os.UserHomeDir(); err == nil {
				local = strings.Replace(local, home, "~", 1)
			}
			status := s.Status
			if status == "" {
				status = "unknown"
			}
			b.WriteString(fmt.Sprintf("  %s%-30s  %s\n", cursor, local, helpStyle.Render(status)))
		}
		b.WriteString("\n")
	}
	b.WriteString("  [a] add  [d] remove\n")

	b.WriteString("\n")

	// Session defaults
	b.WriteString("  Session Defaults\n\n")

	model := cfg.Claude.Model
	if model == "" {
		model = "sonnet"
	}
	effort := cfg.Claude.Effort
	if effort == "" {
		effort = "high"
	}
	teams := "OFF"
	if cfg.Claude.Teams {
		teams = "ON"
	}
	passthrough := "OFF"
	if cfg.Tmux.Passthrough {
		passthrough = "ON"
	}

	b.WriteString(fmt.Sprintf("  Model              %s  [m]\n", stepActiveStyle.Render(model)))
	b.WriteString(fmt.Sprintf("  Effort             %s  [e]\n", stepActiveStyle.Render(effort)))
	b.WriteString(fmt.Sprintf("  Agent Teams        %s  [t]\n", stepActiveStyle.Render(teams)))
	b.WriteString(fmt.Sprintf("  Clickable Links    %s  [l]\n", stepActiveStyle.Render(passthrough)))

	b.WriteString("\n")
	b.WriteString("  [esc] back\n")

	return b.String()
}

func (m settingsModel) viewSyncAdd(b *strings.Builder) string {
	b.WriteString("  Choose a local folder to sync to the server.\n")
	b.WriteString("  Use [→] to browse inside, [enter] to start syncing.\n\n")

	if path := m.browser.DisplayPath(); path != "" {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  %s", path)))
		b.WriteString("\n")
	}

	b.WriteString(m.browser.ViewFilter())
	b.WriteString("\n")

	syncedPaths := make(map[string]bool)
	for _, s := range m.sessions {
		syncedPaths[filepath.Base(s.Local)] = true
	}

	syncIndicator := func(e dirBrowserEntry) string {
		if syncedPaths[e.name] && m.browser.AtRoot() {
			return stepDoneStyle.Render("✓") + " "
		}
		return "  "
	}
	b.WriteString(m.browser.ViewEntries(syncIndicator))

	b.WriteString("\n")
	help := "  [enter] sync this folder  [→] open  [/] filter"
	if !m.browser.AtRoot() {
		help += "  [←] up"
	} else {
		help += "  [esc] cancel"
	}
	b.WriteString("  " + help + "\n")

	return b.String()
}

// cycleValue returns the next value in a list, wrapping around.
func cycleValue(current string, values []string) string {
	for i, v := range values {
		if v == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

// saveDefaultsCmd writes session defaults to the server.
func saveDefaultsCmd(runner interface{ Run(string) ([]byte, error) }, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		// Save locally
		if err := config.Save(*cfg); err != nil {
			return defaultsSavedMsg{err: err}
		}

		// Write defaults.conf on server
		var lines []string
		if cfg.Claude.Model != "" {
			lines = append(lines, "model="+cfg.Claude.Model)
		}
		if cfg.Claude.Effort != "" {
			lines = append(lines, "effort="+cfg.Claude.Effort)
		}
		content := strings.Join(lines, "\n")
		cmd := fmt.Sprintf("cat > ~/.config/fusebox/defaults.conf << 'EOF'\n%s\nEOF", content)
		runner.Run(cmd)

		// Apply passthrough on server
		if cfg.Tmux.Passthrough {
			runner.Run("tmux set -g allow-passthrough on 2>/dev/null; grep -q allow-passthrough ~/.tmux.conf 2>/dev/null || echo 'set -g allow-passthrough on' >> ~/.tmux.conf")
		}

		return defaultsSavedMsg{}
	}
}

// Commands

func loadSyncSessionsCmd(mgr *syncpkg.Manager) tea.Cmd {
	return func() tea.Msg {
		sessions, err := mgr.List()
		return syncSessionsLoadedMsg{sessions: sessions, err: err}
	}
}

func syncAddCmd(mgr *syncpkg.Manager, path string) tea.Cmd {
	return func() tea.Msg {
		err := mgr.Add(path)
		return syncAddedMsg{path: path, err: err}
	}
}

func syncRemoveCmd(mgr *syncpkg.Manager, path string) tea.Cmd {
	return func() tea.Msg {
		err := mgr.Remove(path)
		return syncRemovedMsg{name: path, err: err}
	}
}

func scanLocalDirsCmd(path string) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(path)
		if err != nil {
			return localDirsLoadedMsg{err: err}
		}

		var dirs []dirBrowserEntry
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			count := 0
			subPath := filepath.Join(path, e.Name())
			if subs, err := os.ReadDir(subPath); err == nil {
				for _, s := range subs {
					if s.IsDir() && !strings.HasPrefix(s.Name(), ".") {
						count++
					}
				}
			}
			dirs = append(dirs, dirBrowserEntry{name: e.Name(), count: count})
		}

		return localDirsLoadedMsg{entries: dirs}
	}
}
