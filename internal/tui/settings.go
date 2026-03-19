package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

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

// settingsSection tracks which section is active.
type settingsSection int

const (
	sectionSyncedFolders settingsSection = iota
	sectionTeams
	sectionCount // sentinel for wrapping
)

// settingsModel manages the settings view.
type settingsModel struct {
	section settingsSection

	// Synced folders
	sessions    []syncpkg.SyncSession
	syncCursor  int
	syncConfirm string // non-empty = confirming remove
	syncErr     error
	adding      bool
	browser     dirBrowser
	mgr         *syncpkg.Manager

	// Teams
	teamsEnabled bool
}

func newSettingsModel(mgr *syncpkg.Manager, teamsEnabled bool) settingsModel {
	home, _ := os.UserHomeDir()
	return settingsModel{
		mgr:          mgr,
		browser:      newDirBrowser(home),
		teamsEnabled: teamsEnabled,
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
	}

	return m, nil
}

func (m settingsModel) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  Settings  "))
	b.WriteString("\n\n")

	if m.adding {
		return m.viewSyncAdd(&b)
	}

	// Section: Synced Folders
	m.renderSectionHeader(&b, sectionSyncedFolders, "Synced Folders")
	if m.section == sectionSyncedFolders {
		b.WriteString("  Files sync both ways — your IDE and Claude see the same code.\n\n")

		if m.syncErr != nil {
			b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %v", m.syncErr)))
			b.WriteString("\n\n")
		}

		if len(m.sessions) == 0 {
			b.WriteString("  No folders synced yet. Press [a] to add one.\n\n")
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

		if m.syncConfirm != "" {
			b.WriteString(errorStyle.Render(fmt.Sprintf("  Remove sync %q? [y/n]", m.syncConfirm)))
			b.WriteString("\n\n")
		}

		b.WriteString("  [a] add  [d] remove\n")
	}
	b.WriteString("\n")

	// Section: Teams
	m.renderSectionHeader(&b, sectionTeams, "Agent Teams")
	if m.section == sectionTeams {
		ind := checkOff.String()
		if m.teamsEnabled {
			ind = checkOn.String()
		}
		b.WriteString(fmt.Sprintf("  %s  Enable agent teams for new sessions  [space]\n", ind))
		b.WriteString("  " + helpStyle.Render("Teams let Claude spawn parallel teammates for complex tasks.") + "\n")
	}
	b.WriteString("\n")

	// Footer
	b.WriteString("  [tab] next section  [esc] back\n")

	return b.String()
}

func (m settingsModel) renderSectionHeader(b *strings.Builder, section settingsSection, label string) {
	if m.section == section {
		b.WriteString(stepActiveStyle.Render(fmt.Sprintf("  ● %s", label)))
	} else {
		b.WriteString(helpStyle.Render(fmt.Sprintf("    %s", label)))
	}
	b.WriteString("\n")
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
