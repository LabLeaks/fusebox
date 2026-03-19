package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	syncpkg "github.com/lableaks/fusebox/internal/sync"
)

// Messages for sync view.

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

// syncModel manages the sync view.
type syncModel struct {
	sessions []syncpkg.SyncSession
	cursor   int
	confirm  string // non-empty = confirming remove
	err      error

	// add mode
	adding  bool
	browser dirBrowser

	mgr *syncpkg.Manager
}

func newSyncModel(mgr *syncpkg.Manager) syncModel {
	home, _ := os.UserHomeDir()
	return syncModel{
		mgr:     mgr,
		browser: newDirBrowser(home),
	}
}

func (m syncModel) Update(msg tea.Msg) (syncModel, tea.Cmd) {
	switch msg := msg.(type) {
	case syncSessionsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.sessions = msg.sessions
			m.err = nil
		}
		if m.cursor >= len(m.sessions) {
			m.cursor = max(0, len(m.sessions)-1)
		}
		return m, nil

	case syncAddedMsg:
		m.adding = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
		}
		return m, loadSyncSessionsCmd(m.mgr)

	case syncRemovedMsg:
		m.confirm = ""
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
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

func (m syncModel) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  Synced Folders  "))
	b.WriteString("\n\n")

	if m.adding {
		return m.viewAdd(&b)
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	if len(m.sessions) == 0 {
		b.WriteString("  No folders synced yet. Press [a] to add one.\n\n")
	} else {
		for i, s := range m.sessions {
			cursor := "  "
			if i == m.cursor {
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

	if m.confirm != "" {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Remove sync %q? [y/n]", m.confirm)))
		b.WriteString("\n\n")
	}

	help := "  [a] add folder  [d] remove  [esc] back"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m syncModel) viewAdd(b *strings.Builder) string {
	b.WriteString("  Pick a folder to sync:\n")
	if path := m.browser.DisplayPath(); path != "" {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  %s", path)))
		b.WriteString("\n")
	}

	b.WriteString(m.browser.ViewFilter())
	b.WriteString("\n")

	noIndicator := func(dirBrowserEntry) string { return "" }
	b.WriteString(m.browser.ViewEntries(noIndicator))

	b.WriteString("\n")
	help := "  [enter] sync this folder  [→] open  [/] filter"
	if !m.browser.AtRoot() {
		help += "  [←] up"
	} else {
		help += "  [esc] cancel"
	}
	b.WriteString(helpStyle.Render(help))

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

// scanLocalDirsCmd scans a local directory for non-hidden subdirectories.
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
			// Count subdirectories
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
