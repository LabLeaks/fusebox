package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"

	"github.com/lableaks/fusebox/internal/session"
)

// dirBrowserEntry is a directory with its subdirectory count.
type dirBrowserEntry struct {
	name  string
	count int
}

// dirBrowserAction is returned by Update to tell the parent what happened.
type dirBrowserAction int

const (
	dirBrowserNone     dirBrowserAction = iota
	dirBrowserDrillIn                          // drilled into a subdir — parent should load subdirs
	dirBrowserDrillUp                          // went up a level — parent should load subdirs (or restore root)
	dirBrowserAtRoot                           // esc pressed at root — parent decides what to do
	dirBrowserSelected                         // space pressed — parent handles the selection
	dirBrowserResume                           // r pressed — parent handles resume
	dirBrowserConfirm                          // tab pressed — parent handles confirmation
)

// dirBrowser is a reusable directory navigation component with filter.
type dirBrowser struct {
	entries     []dirBrowserEntry
	rootEntries []dirBrowserEntry // saved top-level entries for restoring
	filtered    []dirBrowserEntry
	cursor      int

	absPath string // current absolute path
	homeDir string // home dir for display

	filter   textinput.Model
	filtering bool // true when filter input is active

	scanning bool
	err      error
}

func newDirBrowser(homeDir string) dirBrowser {
	fi := textinput.New()
	fi.Placeholder = "type to filter..."
	fi.SetWidth(30)

	return dirBrowser{
		homeDir: homeDir,
		absPath: homeDir,
		filter:  fi,
	}
}

// SetEntries sets the directory entries and clears filter.
func (b *dirBrowser) SetEntries(entries []dirBrowserEntry) {
	b.entries = entries
	b.filtered = entries
	b.scanning = false
	b.err = nil
	b.clearFilter()
}

// SetRootEntries saves entries as root (for restore on drill-up to root).
func (b *dirBrowser) SetRootEntries(entries []dirBrowserEntry) {
	b.rootEntries = entries
}

// SetError sets an error state.
func (b *dirBrowser) SetError(err error) {
	b.scanning = false
	b.err = err
}

// SelectedEntry returns the entry at the cursor, if any.
func (b *dirBrowser) SelectedEntry() (dirBrowserEntry, bool) {
	if b.cursor < 0 || b.cursor >= len(b.filtered) {
		return dirBrowserEntry{}, false
	}
	return b.filtered[b.cursor], true
}

// SelectedFullPath returns the absolute path of the selected entry.
func (b *dirBrowser) SelectedFullPath() string {
	e, ok := b.SelectedEntry()
	if !ok {
		return ""
	}
	return b.absPath + "/" + e.name
}

// RelPath returns the path relative to homeDir (for display).
func (b *dirBrowser) RelPath() string {
	if b.absPath == b.homeDir {
		return ""
	}
	return strings.TrimPrefix(b.absPath, b.homeDir+"/")
}

// DisplayPath returns ~/relative for display, empty at root.
func (b *dirBrowser) DisplayPath() string {
	rel := b.RelPath()
	if rel == "" {
		return ""
	}
	return "~/" + rel
}

// AtRoot returns true if at the top-level directory.
func (b *dirBrowser) AtRoot() bool {
	return b.absPath == b.homeDir
}

func (b *dirBrowser) clearFilter() {
	b.filter.SetValue("")
	b.filter.Blur()
	b.filtering = false
	b.filtered = b.entries
	b.cursor = 0
}

func (b *dirBrowser) applyFilter() {
	q := strings.ToLower(b.filter.Value())
	if q == "" {
		b.filtered = b.entries
	} else {
		b.filtered = nil
		for _, e := range b.entries {
			if strings.Contains(strings.ToLower(e.name), q) {
				b.filtered = append(b.filtered, e)
			}
		}
	}
	if b.cursor >= len(b.filtered) {
		b.cursor = len(b.filtered) - 1
	}
	if b.cursor < 0 {
		b.cursor = 0
	}
}

// Update handles key events. Returns the action for the parent to handle.
func (b *dirBrowser) Update(msg tea.Msg) (dirBrowserAction, tea.Cmd) {
	kmsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return dirBrowserNone, nil
	}

	key := kmsg.String()

	// If filtering, route most keys to the filter input
	if b.filtering {
		switch key {
		case keyEsc:
			b.clearFilter()
			return dirBrowserNone, nil
		case keyAttach: // enter confirms filter, selects top match
			b.filter.Blur()
			b.filtering = false
			return dirBrowserNone, nil
		case "up":
			if b.cursor > 0 {
				b.cursor--
			}
			return dirBrowserNone, nil
		case "down":
			if b.cursor < len(b.filtered)-1 {
				b.cursor++
			}
			return dirBrowserNone, nil
		}
		// All other keys go to the filter textinput
		var cmd tea.Cmd
		b.filter, cmd = b.filter.Update(msg)
		b.applyFilter()
		return dirBrowserNone, cmd
	}

	// Normal (non-filtering) mode
	switch key {
	case "up":
		if b.cursor > 0 {
			b.cursor--
		}
		return dirBrowserNone, nil
	case "down":
		if b.cursor < len(b.filtered)-1 {
			b.cursor++
		}
		return dirBrowserNone, nil
	case keyAttach, "right": // drill down
		if e, ok := b.SelectedEntry(); ok && e.count > 0 {
			b.absPath = b.absPath + "/" + e.name
			b.cursor = 0
			b.scanning = true
			b.clearFilter()
			return dirBrowserDrillIn, nil
		}
		return dirBrowserNone, nil
	case "left": // go up
		if !b.AtRoot() {
			return b.drillUp()
		}
		return dirBrowserNone, nil
	case keyEsc:
		if !b.AtRoot() {
			return b.drillUp()
		}
		return dirBrowserAtRoot, nil
	case "space":
		return dirBrowserSelected, nil
	case "r":
		return dirBrowserResume, nil
	case "tab":
		return dirBrowserConfirm, nil
	case "/": // activate filter
		b.filtering = true
		b.filter.Focus()
		return dirBrowserNone, nil
	}

	return dirBrowserNone, nil
}

func (b *dirBrowser) drillUp() (dirBrowserAction, tea.Cmd) {
	b.absPath = b.absPath[:strings.LastIndex(b.absPath, "/")]
	b.cursor = 0
	b.clearFilter()
	if b.AtRoot() && b.rootEntries != nil {
		b.entries = b.rootEntries
		b.filtered = b.rootEntries
		return dirBrowserDrillUp, nil
	}
	b.scanning = true
	return dirBrowserDrillUp, nil
}

// HandleSubdirsLoaded processes a subdirs response from the server.
func (b *dirBrowser) HandleSubdirsLoaded(entries []session.SubdirEntry, err error) {
	b.scanning = false
	if err != nil {
		b.err = err
		return
	}
	b.entries = nil
	for _, e := range entries {
		b.entries = append(b.entries, dirBrowserEntry{name: e.Path, count: e.Count})
	}
	b.filtered = b.entries
	b.err = nil
	b.cursor = 0
}

// ViewEntries renders the directory list portion.
func (b *dirBrowser) ViewEntries(isSelected func(entry dirBrowserEntry) string) string {
	var sb strings.Builder

	if b.scanning {
		sb.WriteString("  Loading...\n")
		return sb.String()
	}

	if b.err != nil {
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %v", b.err)))
		sb.WriteString("\n\n")
	}

	if len(b.filtered) == 0 {
		if b.filter.Value() != "" {
			sb.WriteString("  No matches.\n")
		} else {
			sb.WriteString("  (no subdirectories)\n")
		}
		return sb.String()
	}

	visible := b.filtered
	maxVisible := 15
	offset := 0
	if b.cursor >= maxVisible {
		offset = b.cursor - maxVisible + 1
	}
	if offset+maxVisible < len(visible) {
		visible = visible[offset : offset+maxVisible]
	} else if offset < len(visible) {
		visible = visible[offset:]
	}

	for i, d := range visible {
		idx := offset + i
		cursor := "  "
		if idx == b.cursor {
			cursor = "▸ "
		}

		indicator := isSelected(d)

		detail := ""
		if d.count > 0 {
			detail = helpStyle.Render(fmt.Sprintf("  %d subdirs", d.count))
		}
		sb.WriteString(fmt.Sprintf("  %s%s%s%s%s\n",
			cursor, indicator, d.name,
			strings.Repeat(" ", max(1, 24-len(d.name))),
			detail,
		))
	}

	if len(b.filtered) > maxVisible {
		sb.WriteString(fmt.Sprintf("\n  (%d/%d)", len(visible), len(b.filtered)))
	}

	return sb.String()
}

// ViewFilter renders the filter input if active.
func (b *dirBrowser) ViewFilter() string {
	if !b.filtering {
		return ""
	}
	return fmt.Sprintf("  %s %s\n", filterPromptStyle.Render("/"), b.filter.View())
}
