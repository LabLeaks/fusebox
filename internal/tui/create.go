package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/lableaks/fusebox/internal/session"
)

// Messages for create view async operations.

type subdirsLoadedMsg struct {
	entries []session.SubdirEntry
	err     error
}

type createModel struct {
	browser      dirBrowser
	teamsEnabled bool
}

func newCreate(dirs []string, homeDir string) createModel {
	// Convert flat dir list (from roots.conf) into root-level entries.
	seen := make(map[string]bool)
	var entries []dirBrowserEntry
	rootCounts := make(map[string]int)

	for _, d := range dirs {
		rel := strings.TrimPrefix(d, homeDir+"/")
		parts := strings.SplitN(rel, "/", 2)
		rootCounts[parts[0]]++
	}

	for _, d := range dirs {
		rel := strings.TrimPrefix(d, homeDir+"/")
		parts := strings.SplitN(rel, "/", 2)
		root := parts[0]
		if !seen[root] {
			seen[root] = true
			entries = append(entries, dirBrowserEntry{name: root, count: rootCounts[root]})
		}
	}

	b := newDirBrowser(homeDir)
	b.SetEntries(entries)
	b.SetRootEntries(entries)

	return createModel{browser: b}
}

func (m createModel) selectedDir() string {
	return m.browser.SelectedFullPath()
}

func (m createModel) sessionName() string {
	dir := m.selectedDir()
	if dir == "" {
		return ""
	}
	return filepath.Base(dir)
}

func (m createModel) Update(msg tea.Msg) (createModel, tea.Cmd) {
	if sdm, ok := msg.(subdirsLoadedMsg); ok {
		m.browser.HandleSubdirsLoaded(sdm.entries, sdm.err)
		return m, nil
	}

	action, cmd := m.browser.Update(msg)
	_ = action // parent (app.go updateCreate) handles actions
	return m, cmd
}

func (m createModel) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("  Create Session  "))
	b.WriteString("\n\n")

	if path := m.browser.DisplayPath(); path != "" {
		b.WriteString("  ")
		b.WriteString(helpStyle.Render(path))
		b.WriteString("\n")
	}

	b.WriteString(m.browser.ViewFilter())
	b.WriteString("\n")

	noIndicator := func(dirBrowserEntry) string { return "" }
	b.WriteString(m.browser.ViewEntries(noIndicator))

	// Options
	teamsLabel := "OFF"
	if m.teamsEnabled {
		teamsLabel = "ON"
	}
	b.WriteString(fmt.Sprintf("\n  Teams: %s\n", teamsLabel))

	b.WriteString("\n")
	var help string
	if e, ok := m.browser.SelectedEntry(); ok {
		if e.count > 0 {
			help = fmt.Sprintf("  [→] open  [n] new  [r] resume \"%s\"", e.name)
		} else {
			help = fmt.Sprintf("  [n] new  [r] resume \"%s\"", e.name)
		}
	}
	help += "  [t] teams  [/] filter"
	if !m.browser.AtRoot() {
		help += "  [←] up"
	} else {
		help += "  [esc] cancel"
	}
	b.WriteString(helpStyle.Render(help))

	return b.String()
}
