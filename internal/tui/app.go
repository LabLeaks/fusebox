package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/session"
	"github.com/lableaks/fusebox/internal/ssh"
	syncpkg "github.com/lableaks/fusebox/internal/sync"
)

type view int

const (
	viewDashboard view = iota
	viewCreate
	viewTeamDetail
	viewSettings
)

// Messages

type sessionsLoadedMsg struct {
	sessions []session.Session
	err      error
}

type dirsLoadedMsg struct {
	dirs    []string
	counts  map[string]int // subdir counts keyed by dir path
	err     error
	homeDir string // override homeDir for display (e.g. ~/.fusebox/sync)
}

type sessionCreatedMsg struct {
	err error
}

type sessionStoppedMsg struct {
	name string
	err  error
}

type attachFinishedMsg struct {
	err error
}

type mutagenStatusMsg struct {
	status string
	err    error
}


type previewLoadedMsg struct {
	content string
	name    string
	err     error
}

type previewTickMsg time.Time

type activityLoadedMsg struct {
	activity map[string]session.ToolActivity
	err      error
}

type activityTickMsg time.Time
type mutagenTickMsg time.Time

type teamsLoadedMsg struct {
	teams []session.TeamStatus
	err   error
}

type panesLoadedMsg struct {
	panes   []session.PaneInfo
	session string
	err     error
}

type panePreviewLoadedMsg struct {
	content string
	pane    int
	err     error
}

// Model

type Model struct {
	cfg          config.Config
	ssh          ssh.Runner
	manager      *session.Manager
	view         view
	dashboard    dashboardModel
	create       createModel
	activity     map[string]session.ToolActivity
	mutagen      string
	width        int
	height       int
	loading      bool
	loadingDirs  bool
	spinner      spinner.Model
	showPreview  bool
	preview      viewport.Model
	previewName  string
	pendingErr   error // preserved across dashboard rebuilds (e.g. create errors)
	teams        []session.TeamStatus
	teamSessions map[string]string // team name → session name
	teamDetail   teamDetailModel
	teamPanes    map[string]int // session name → pane count (for team detection)
	settings     settingsModel
	syncMgr      *syncpkg.Manager
}

func New(cfg config.Config) Model {
	client := ssh.NewClient(cfg.Server.Host, cfg.Server.User)
	return NewWithRunner(cfg, client)
}

func NewWithRunner(cfg config.Config, runner ssh.Runner) Model {
	s := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(spinnerStyle),
	)
	var sm *syncpkg.Manager
	if !cfg.IsLocal() {
		sm = syncpkg.NewManager(cfg.ResolveSandboxDataDir(), cfg.SSHTarget())
	}

	host := cfg.Server.Host
	if cfg.IsLocal() {
		host, _ = os.Hostname()
	}

	serverPath := cfg.ResolveServerPath()
	if cfg.IsLocal() {
		// For local mode, find our own executable
		if exe, err := os.Executable(); err == nil {
			serverPath = exe
		}
	}

	return Model{
		cfg:       cfg,
		ssh:       runner,
		manager:   session.NewManager(runner, serverPath),
		dashboard: newDashboard(nil, host, cfg.ResolveHomeDir()),
		preview:   viewport.New(),
		loading:   true,
		spinner:   s,
		syncMgr:   sm,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadSessionsCmd(m.manager),
		loadActivityCmd(m.manager),
		loadTeamsCmd(m.manager),
		loadMutagenCmd(),
		m.spinner.Tick,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeLayout()
		// Reserve lines for header, help, etc. (~10 lines)
		if avail := m.height - 10; avail > 3 {
			m.create.browser.maxVisible = avail
		}
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case keyCtrlC:
			if m.view == viewCreate || m.view == viewSettings {
				m.view = viewDashboard
				m.loadingDirs = false
				return m, nil
			}
			return m, tea.Quit
		case keyQuit:
			if m.view == viewDashboard {
				return m, tea.Quit
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case sessionsLoadedMsg:
		m.loading = false
		m.dashboard = newDashboard(msg.sessions, m.cfg.Server.Host, m.cfg.ResolveHomeDir())
		if msg.err != nil {
			m.dashboard.err = msg.err
		} else if m.pendingErr != nil {
			m.dashboard.err = m.pendingErr
			m.pendingErr = nil
		}
		// Re-apply existing activity after dashboard rebuild
		if m.activity != nil {
			m.dashboard.updateActivity(m.activity)
		}
		// Re-apply team data after dashboard rebuild
		if m.teamSessions != nil {
			m.dashboard.updateTeams(m.teamSessions, m.teams)
		}
		m.resizeLayout()
		var cmds []tea.Cmd
		if m.showPreview {
			if s, ok := m.dashboard.selectedSession(); ok {
				cmds = append(cmds, loadPreviewCmd(m.manager, s.Name))
			}
		}
		// If teams loaded but panes haven't been fetched yet, fetch them now
		if len(m.teams) > 0 && len(m.dashboard.sessions) > 0 {
			for _, s := range m.dashboard.sessions {
				if _, hasPanes := m.teamPanes[s.Name]; !hasPanes {
					cmds = append(cmds, loadPanesCmd(m.manager, s.Name))
				}
			}
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case activityLoadedMsg:
		if msg.err == nil {
			m.activity = msg.activity
			m.dashboard.updateActivity(m.activity)
		}
		// Schedule next activity tick regardless of error
		return m, activityTickCmd()

	case activityTickMsg:
		return m, tea.Batch(loadActivityCmd(m.manager), loadTeamsCmd(m.manager))

	case dirsLoadedMsg:
		m.loadingDirs = false
		if msg.err != nil {
			m.dashboard.err = msg.err
			m.view = viewDashboard
			return m, nil
		}
		if len(msg.dirs) == 0 {
			m.dashboard.err = fmt.Errorf("no folders on server yet — mutagen is still syncing. Wait for sync to complete and try again")
			m.view = viewDashboard
			return m, nil
		}
		homeDir := m.cfg.ResolveHomeDir()
		if msg.homeDir != "" {
			homeDir = msg.homeDir
		}
		if msg.counts != nil {
			m.create = newCreateWithCounts(msg.dirs, homeDir, msg.counts)
		} else {
			m.create = newCreate(msg.dirs, homeDir)
		}
		m.view = viewCreate
		return m, nil

	case subdirsLoadedMsg:
		if m.view == viewCreate {
			m.create, _ = m.create.Update(msg)
		}
		return m, nil

	case sessionCreatedMsg:
		if msg.err != nil {
			m.pendingErr = msg.err
			m.dashboard.err = msg.err
		}
		return m, loadSessionsCmd(m.manager)

	case sessionStoppedMsg:
		if msg.err != nil {
			m.dashboard.err = msg.err
			// Stop failed — refetch to restore the session
			return m, loadSessionsCmd(m.manager)
		}
		// Optimistic removal already done; refetch to sync
		return m, loadSessionsCmd(m.manager)

	case attachFinishedMsg:
		return m, loadSessionsCmd(m.manager)

	case stopRequestMsg:
		// Optimistic: show "stopping..." state immediately
		m.dashboard.removeSession(msg.name)
		name := msg.name
		return m, func() tea.Msg {
			err := m.manager.Stop(name)
			return sessionStoppedMsg{name: name, err: err}
		}

	case mutagenStatusMsg:
		if msg.err != nil {
			m.mutagen = mutagenErr.Render("sync: " + msg.status)
		} else if msg.status != "" {
			m.mutagen = mutagenOK.Render("sync: " + msg.status)
		}
		// Poll every 5s to track sync progress
		return m, tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return mutagenTickMsg(t) })

	case mutagenTickMsg:
		return m, loadMutagenCmd()


	case previewLoadedMsg:
		if !m.showPreview {
			return m, nil
		}
		if msg.err != nil {
			m.preview.SetContent(errorStyle.Render(fmt.Sprintf("Error: %v", msg.err)))
		} else {
			m.preview.SetContent(msg.content)
			m.preview.GotoBottom()
		}
		m.previewName = msg.name
		return m, previewTickCmd()

	case previewTickMsg:
		if !m.showPreview {
			return m, nil
		}
		if s, ok := m.dashboard.selectedSession(); ok {
			return m, loadPreviewCmd(m.manager, s.Name)
		}
		return m, nil

	case teamsLoadedMsg:
		if msg.err == nil && len(msg.teams) > 0 {
			m.teams = msg.teams
			m.detectTeamSessions()
			m.dashboard.updateTeams(m.teamSessions, m.teams)
		} else if msg.err == nil {
			m.teams = nil
			m.teamSessions = nil
			m.dashboard.updateTeams(nil, nil)
		}
		// After teams load, fetch pane counts for sessions to detect leads
		if len(m.dashboard.sessions) > 0 && len(m.teams) > 0 {
			var cmds []tea.Cmd
			for _, s := range m.dashboard.sessions {
				cmds = append(cmds, loadPanesCmd(m.manager, s.Name))
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case panesLoadedMsg:
		if msg.err == nil {
			if m.teamPanes == nil {
				m.teamPanes = make(map[string]int)
			}
			m.teamPanes[msg.session] = len(msg.panes)
			m.detectTeamSessions()
			m.dashboard.updateTeams(m.teamSessions, m.teams)

			// If this is for the team detail view, update it
			if m.view == viewTeamDetail && m.teamDetail.session == msg.session {
				m.teamDetail.panes = msg.panes
			}
		}
		return m, nil

	case panePreviewLoadedMsg:
		if m.view != viewTeamDetail || !m.teamDetail.showPreview {
			return m, nil
		}
		if msg.err != nil {
			m.teamDetail.preview.SetContent(errorStyle.Render(fmt.Sprintf("Error: %v", msg.err)))
		} else {
			m.teamDetail.preview.SetContent(msg.content)
			m.teamDetail.preview.GotoBottom()
		}
		return m, nil
	}

	// Route to active view
	switch m.view {
	case viewDashboard:
		return m.updateDashboard(msg)
	case viewCreate:
		return m.updateCreate(msg)
	case viewTeamDetail:
		return m.updateTeamDetail(msg)
	case viewSettings:
		return m.updateSettings(msg)
	}

	return m, nil
}

func (m Model) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case keyPreview:
			return m.togglePreview()
		case keyEsc:
			if m.showPreview {
				m.showPreview = false
				m.resizeLayout()
				return m, nil
			}
		case keyNew:
			m.loadingDirs = true
			return m, loadDirsCmd(m.manager)
		case keySync:
			m.settings = newSettingsModel(m.syncMgr)
			m.view = viewSettings
			return m, loadSyncSessionsCmd(m.syncMgr)
		case keyAttach:
			if s, ok := m.dashboard.selectedSession(); ok {
				cmd := m.ssh.AttachCmd(s.Name)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return attachFinishedMsg{err: err}
				})
			}
			return m, nil
		case keyTeams:
			if s, ok := m.dashboard.selectedSession(); ok {
				if teamName, isLead := m.sessionIsTeamLead(s.Name); isLead {
					// Find the team status
					for _, ts := range m.teams {
						if ts.Name == teamName {
							m.teamDetail = newTeamDetail(ts, nil, s.Name)
							m.view = viewTeamDetail
							return m, loadPanesCmd(m.manager, s.Name)
						}
					}
				}
			}
			return m, nil
		}
	}

	prevCursor := m.dashboard.table.Cursor()
	var cmd tea.Cmd
	m.dashboard, cmd = m.dashboard.Update(msg)

	// If cursor moved while preview is visible, fetch new preview
	if m.showPreview && m.dashboard.table.Cursor() != prevCursor {
		if s, ok := m.dashboard.selectedSession(); ok {
			return m, tea.Batch(cmd, loadPreviewCmd(m.manager, s.Name))
		}
	}

	return m, cmd
}

func (m Model) togglePreview() (tea.Model, tea.Cmd) {
	if len(m.dashboard.sessions) == 0 {
		return m, nil
	}
	m.showPreview = !m.showPreview
	m.resizeLayout()
	if m.showPreview {
		if s, ok := m.dashboard.selectedSession(); ok {
			return m, loadPreviewCmd(m.manager, s.Name)
		}
	}
	return m, nil
}

func (m *Model) resizeLayout() {
	const overhead = 7 // header + spacing + help + mutagen + padding
	available := m.height - overhead
	if available < 2 {
		available = 2
	}

	if m.showPreview {
		tableH := available / 2
		previewH := available - tableH
		m.dashboard.table.SetHeight(tableH)
		m.preview.SetWidth(m.width - 4) // account for border padding
		m.preview.SetHeight(previewH - 2) // account for header + border
	} else {
		m.dashboard.table.SetHeight(available)
	}
}

func (m Model) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Let the browser handle the message first
	action, browseCmd := m.create.browser.Update(msg)

	switch action {
	case dirBrowserAtRoot:
		m.view = viewDashboard
		return m, nil
	case dirBrowserSelected:
		dir := m.create.selectedDir()
		name := m.create.sessionName()
		if dir != "" && name != "" {
			m.view = viewDashboard
			m.dashboard.addPendingSession(name, dir)
			m.resizeLayout()
			if m.cfg.Claude.Teams {
				return m, createTeamSessionCmd(m.manager, name, dir)
			}
			return m, createSessionCmd(m.manager, name, dir)
		}
		return m, nil
	case dirBrowserResume:
		dir := m.create.selectedDir()
		name := m.create.sessionName()
		if dir != "" && name != "" {
			m.view = viewDashboard
			m.dashboard.addPendingSession(name, dir)
			m.resizeLayout()
			if m.cfg.Claude.Teams {
				return m, createTeamResumeCmd(m.manager, name, dir)
			}
			return m, createResumeCmd(m.manager, name, dir)
		}
		return m, nil
	case dirBrowserDrillIn:
		return m, loadSubdirsCmd(m.manager, m.create.browser.absPath)
	case dirBrowserDrillUp:
		if m.create.browser.scanning {
			return m, loadSubdirsCmd(m.manager, m.create.browser.absPath)
		}
		return m, nil
	}

	// Handle subdirs loaded
	if sdm, ok := msg.(subdirsLoadedMsg); ok {
		m.create.browser.HandleSubdirsLoaded(sdm.entries, sdm.err)
		return m, nil
	}

	return m, browseCmd
}

func (m Model) updateTeamDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case keyEsc:
			if m.teamDetail.showPreview {
				m.teamDetail.showPreview = false
				return m, nil
			}
			m.view = viewDashboard
			return m, nil
		case keyPreview:
			m.teamDetail.showPreview = !m.teamDetail.showPreview
			if m.teamDetail.showPreview {
				paneIdx := m.teamDetail.cursor
				return m, loadPanePreviewCmd(m.manager, m.teamDetail.session, paneIdx)
			}
			return m, nil
		case keyAttach:
			paneIdx := m.teamDetail.cursor
			cmd := m.ssh.AttachPaneCmd(m.teamDetail.session, paneIdx)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return attachFinishedMsg{err: err}
			})
		}
	}

	var cmd tea.Cmd
	m.teamDetail, cmd = m.teamDetail.Update(msg)
	return m, cmd
}

func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle key events at this level for navigation
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if m.settings.adding {
			// In add mode — intercept enter to sync (not drill)
			switch kmsg.String() {
			case keyAttach: // enter = sync this folder
				dir := m.settings.browser.SelectedFullPath()
				if dir != "" {
					m.settings.adding = false
					return m, syncAddCmd(m.syncMgr, dir)
				}
				return m, nil
			case keyEsc:
				if m.settings.browser.AtRoot() {
					m.settings.adding = false
					return m, nil
				}
			}

			action, browseCmd := m.settings.browser.Update(msg)
			switch action {
			case dirBrowserAtRoot:
				m.settings.adding = false
				return m, nil
			case dirBrowserDrillIn:
				return m, scanLocalDirsCmd(m.settings.browser.absPath)
			case dirBrowserDrillUp:
				if m.settings.browser.scanning {
					return m, scanLocalDirsCmd(m.settings.browser.absPath)
				}
				return m, nil
			}

			// Pass local dir scan results to sync view
			if ldm, ok := msg.(localDirsLoadedMsg); ok {
				m.settings, _ = m.settings.Update(ldm)
				return m, nil
			}

			return m, browseCmd
		}

		switch kmsg.String() {
		case keyEsc:
			m.view = viewDashboard
			return m, nil
		// Synced folders
		case "a":
			m.settings.adding = true
			home, _ := os.UserHomeDir()
			m.settings.browser = newDirBrowser(home)
			return m, scanLocalDirsCmd(home)
		case keyStop: // d = remove
			if len(m.settings.sessions) > 0 && m.settings.syncConfirm == "" {
				s := m.settings.sessions[m.settings.syncCursor]
				m.settings.syncConfirm = s.Local
			}
			return m, nil
		case "y":
			if m.settings.syncConfirm != "" {
				path := m.settings.syncConfirm
				m.settings.syncConfirm = ""
				return m, syncRemoveCmd(m.syncMgr, path)
			}
			return m, nil
		case "up":
			if m.settings.syncCursor > 0 {
				m.settings.syncCursor--
			}
			return m, nil
		case "down":
			if m.settings.syncCursor < len(m.settings.sessions)-1 {
				m.settings.syncCursor++
			}
			return m, nil
		// Toggles
		case "m":
			current := m.cfg.Claude.Model
			if current == "" {
				current = "sonnet"
			}
			m.cfg.Claude.Model = cycleValue(current, models)
			return m, saveDefaultsCmd(m.ssh, &m.cfg)
		case "e":
			current := m.cfg.Claude.Effort
			if current == "" {
				current = "high"
			}
			m.cfg.Claude.Effort = cycleValue(current, efforts)
			return m, saveDefaultsCmd(m.ssh, &m.cfg)
		case "t":
			m.cfg.Claude.Teams = !m.cfg.Claude.Teams
			return m, saveDefaultsCmd(m.ssh, &m.cfg)
		case "l":
			m.cfg.Tmux.Passthrough = !m.cfg.Tmux.Passthrough
			return m, saveDefaultsCmd(m.ssh, &m.cfg)
		default:
			if m.settings.syncConfirm != "" {
				m.settings.syncConfirm = ""
				return m, nil
			}
		}
	}

	// Pass other messages (loaded, added, removed) to sync model
	var cmd tea.Cmd
	m.settings, cmd = m.settings.Update(msg)
	return m, cmd
}

// detectTeamSessions maps team names to session names.
// A session is considered a team lead if it has >1 pane.
func (m *Model) detectTeamSessions() {
	if len(m.teams) == 0 {
		m.teamSessions = nil
		return
	}
	m.teamSessions = make(map[string]string)
	for _, s := range m.dashboard.sessions {
		paneCount, hasPanes := m.teamPanes[s.Name]
		if hasPanes && paneCount > 1 {
			// Match to a team — use first unmatched team
			for _, ts := range m.teams {
				if _, taken := m.teamSessions[ts.Name]; !taken {
					m.teamSessions[ts.Name] = s.Name
					break
				}
			}
		}
	}
}

// sessionIsTeamLead returns the team name if the session is a team lead.
func (m *Model) sessionIsTeamLead(sessionName string) (string, bool) {
	for teamName, sName := range m.teamSessions {
		if sName == sessionName {
			return teamName, true
		}
	}
	return "", false
}

func (m Model) View() tea.View {
	var b strings.Builder

	switch m.view {
	case viewDashboard:
		if m.loading {
			b.WriteString(headerStyle.Render("  FUSEBOX  ·  " + m.cfg.Server.Host))
			b.WriteString("\n\n")
			b.WriteString(fmt.Sprintf("  %s Connecting to %s...", m.spinner.View(), m.cfg.Server.Host))
			b.WriteString("\n")
		} else {
			b.WriteString(m.dashboard.View())
			if m.loadingDirs {
				b.WriteString("\n")
				b.WriteString(fmt.Sprintf("  %s Loading directories...", m.spinner.View()))
			}
			if m.showPreview {
				b.WriteString("\n")
				b.WriteString(m.renderPreview())
			}
		}
	case viewCreate:
		b.WriteString(m.create.View())
	case viewTeamDetail:
		b.WriteString(m.teamDetail.View())
	case viewSettings:
		b.WriteString(m.settings.View(m.cfg))
	}

	if m.mutagen != "" {
		b.WriteString("\n\n  ")
		b.WriteString(m.mutagen)
	}

	b.WriteString("\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m Model) renderPreview() string {
	name := m.previewName
	if name == "" {
		if s, ok := m.dashboard.selectedSession(); ok {
			name = s.Name
		}
	}

	header := previewHeader.Render(fmt.Sprintf("  Preview: %s", name)) +
		helpStyle.Render("  ↻ 2s")

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.preview.View(),
	)

	return previewBorder.Width(m.width - 4).Render(content)
}

// Commands

func loadSessionsCmd(mgr *session.Manager) tea.Cmd {
	return func() tea.Msg {
		sessions, err := mgr.List()
		return sessionsLoadedMsg{sessions: sessions, err: err}
	}
}

// loadDirsCmd lists synced folders on the server, falling back to browse roots.
// Uses the subdirs command to get entry counts so drill-down works.
func loadDirsCmd(mgr *session.Manager) tea.Cmd {
	return func() tea.Msg {
		// Check for synced folders at ~/.fusebox/sync/
		homeOut, _ := mgr.SSH.Run("echo $HOME")
		home := strings.TrimSpace(string(homeOut))
		syncBase := home + "/.fusebox/sync"

		// Use subdirs to get proper counts for drill-down
		out, err := mgr.SSH.Run(mgr.ServerPath() + " subdirs " + syncBase)
		if err == nil {
			var entries []session.SubdirEntry
			if json.Unmarshal(out, &entries) == nil && len(entries) > 0 {
				var dirs []string
				counts := make(map[string]int)
				for _, e := range entries {
					fullPath := syncBase + "/" + e.Path
					dirs = append(dirs, fullPath)
					counts[e.Path] = e.Count
				}
				return dirsLoadedMsg{dirs: dirs, homeDir: syncBase, counts: counts}
			}
		}
		// Fall back to legacy browse roots
		dirs, lErr := mgr.ListDirs()
		return dirsLoadedMsg{dirs: dirs, err: lErr}
	}
}

func loadSubdirsCmd(mgr *session.Manager, path string) tea.Cmd {
	return func() tea.Msg {
		entries, err := mgr.ListSubdirs(path)
		return subdirsLoadedMsg{entries: entries, err: err}
	}
}

func createSessionCmd(mgr *session.Manager, name, dir string) tea.Cmd {
	return func() tea.Msg {
		err := mgr.Create(name, dir)
		return sessionCreatedMsg{err: err}
	}
}

func stopSessionCmd(name string) tea.Cmd {
	return func() tea.Msg {
		return stopRequestMsg{name: name}
	}
}

type stopRequestMsg struct {
	name string
}

func loadPreviewCmd(mgr *session.Manager, name string) tea.Cmd {
	return func() tea.Msg {
		content, err := mgr.Preview(name, 30)
		return previewLoadedMsg{content: content, name: name, err: err}
	}
}

func previewTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return previewTickMsg(t)
	})
}

func loadActivityCmd(mgr *session.Manager) tea.Cmd {
	return func() tea.Msg {
		activity, err := mgr.FetchActivity()
		return activityLoadedMsg{activity: activity, err: err}
	}
}

func activityTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return activityTickMsg(t)
	})
}

func createResumeCmd(mgr *session.Manager, name, dir string) tea.Cmd {
	return func() tea.Msg {
		err := mgr.CreateResume(name, dir)
		return sessionCreatedMsg{err: err}
	}
}

func createTeamResumeCmd(mgr *session.Manager, name, dir string) tea.Cmd {
	return func() tea.Msg {
		err := mgr.CreateTeamResume(name, dir)
		return sessionCreatedMsg{err: err}
	}
}

func createTeamSessionCmd(mgr *session.Manager, name, dir string) tea.Cmd {
	return func() tea.Msg {
		err := mgr.CreateTeam(name, dir)
		return sessionCreatedMsg{err: err}
	}
}

func loadTeamsCmd(mgr *session.Manager) tea.Cmd {
	return func() tea.Msg {
		teams, err := mgr.ListTeams()
		return teamsLoadedMsg{teams: teams, err: err}
	}
}

func loadPanesCmd(mgr *session.Manager, sessionName string) tea.Cmd {
	return func() tea.Msg {
		panes, err := mgr.ListPanes(sessionName)
		return panesLoadedMsg{panes: panes, session: sessionName, err: err}
	}
}

func loadPanePreviewCmd(mgr *session.Manager, sessionName string, pane int) tea.Cmd {
	return func() tea.Msg {
		content, err := mgr.PanePreview(sessionName, pane, 30)
		return panePreviewLoadedMsg{content: content, pane: pane, err: err}
	}
}

// mutagenHumanize translates mutagen status jargon into plain English.
func mutagenHumanize(s string) string {
	replacer := strings.NewReplacer(
		"Watching for changes", "ready",
		"Scanning files", "scanning",
		"Staging files on alpha", "uploading",
		"Staging files on beta", "uploading to server",
		"Reconciling changes", "syncing",
		"Saving archive", "finalizing",
		"Halted", "halted",
	)
	return replacer.Replace(s)
}

func loadMutagenCmd() tea.Cmd {
	return func() tea.Msg {
		if _, err := exec.LookPath("mutagen"); err != nil {
			return mutagenStatusMsg{} // not installed, skip silently
		}
		out, err := exec.Command("mutagen", "sync", "list",
			"--label-selector", "fusebox=true",
			"--template", `{{range .}}{{.Name}}: {{.Status.Description}}{{"\n"}}{{end}}`).Output()
		if err != nil {
			return mutagenStatusMsg{err: err}
		}
		output := strings.TrimSpace(string(out))
		if output == "" {
			return mutagenStatusMsg{}
		}

		var summary []string
		hasError := false
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Strip "fusebox-" prefix from name
			line = strings.TrimPrefix(line, "fusebox-")
			// Translate mutagen jargon into plain English
			line = mutagenHumanize(line)
			if strings.Contains(line, "error") || strings.Contains(line, "halted") {
				hasError = true
			}
			summary = append(summary, line)
		}

		result := strings.Join(summary, " · ")
		if hasError {
			return mutagenStatusMsg{status: result, err: fmt.Errorf("sync error")}
		}
		return mutagenStatusMsg{status: result}
	}
}
