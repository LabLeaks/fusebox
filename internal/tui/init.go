package tui

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/ssh"
)

type initStep int

const (
	stepMode     initStep = iota // local vs remote
	stepHost     initStep = iota
	stepUser     initStep = iota
	stepConnect  initStep = iota
	stepDirs     initStep = iota
	stepSettings initStep = iota
	stepWrite    initStep = iota
	stepSyncing  initStep = iota
	stepDone     initStep = iota
)

type initConnectSub int

const (
	subSSH     initConnectSub = iota
	subDeploy  initConnectSub = iota
	subDiscover initConnectSub = iota
)

type InitModel struct {
	step       initStep
	localMode  bool
	hostInput  textinput.Model
	userInput  textinput.Model
	host       string
	user       string
	goarch     string
	homeDir    string
	sshFactory func(host, user string) ssh.Runner
	browser    dirBrowser
	selected   map[string]bool // keyed by path relative to homeDir
	spinner    spinner.Model
	progress   string
	connectSub initConnectSub
	err         error
	width       int
	height      int
	launch      bool
	reconfig    bool
	passthrough  bool   // tmux allow-passthrough
	sandboxOK    bool   // server supports sandboxing
	sandboxWhy   string // reason if not supported
	syncTotal    int    // total folders to sync
	syncDone     int    // folders synced so far
	syncCurrent  string // folder currently syncing
	syncErr      error  // last sync error (non-fatal, continues)
}

// NewInit creates an init wizard model. hostArg is the optional user@host argument.
func NewInit(hostArg string) InitModel {
	return NewInitWithSSH(hostArg, defaultSSHFactory)
}

// NewInitWithSSH creates an init wizard with a custom SSH factory (for testing).
func NewInitWithSSH(hostArg string, factory func(host, user string) ssh.Runner) InitModel {
	hi := textinput.New()
	hi.Placeholder = "hostname or IP"
	hi.SetWidth(40)
	hi.CharLimit = 256

	ui := textinput.New()
	ui.Placeholder = "SSH username"
	ui.SetWidth(40)
	ui.CharLimit = 64

	s := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(spinnerStyle),
	)

	m := InitModel{
		step:       stepMode,
		hostInput:  hi,
		userInput:  ui,
		sshFactory: factory,
		spinner:    s,
		selected:   make(map[string]bool),
	}

	// Default user to current OS user
	if u, err := user.Current(); err == nil {
		m.userInput.SetValue(u.Username)
	}

	// Parse user@host from argument — skip mode selection, go straight to remote
	if hostArg != "" {
		if at := strings.Index(hostArg, "@"); at >= 0 {
			m.userInput.SetValue(hostArg[:at])
			m.hostInput.SetValue(hostArg[at+1:])
		} else {
			m.hostInput.SetValue(hostArg)
		}
		// Both provided — skip to connect
		if m.hostInput.Value() != "" && m.userInput.Value() != "" {
			m.host = m.hostInput.Value()
			m.user = m.userInput.Value()
			m.step = stepConnect
			m.connectSub = subSSH
			m.progress = "Testing SSH connection..."
		} else {
			m.step = stepHost
			m.hostInput.Focus()
		}
	}

	// Check existing config for pre-fill
	if cfg, err := config.Load(); err == nil {
		if cfg.Server.Host != "" {
			if m.hostInput.Value() == "" {
				m.hostInput.SetValue(cfg.Server.Host)
			}
			if m.userInput.Value() == "" || m.userInput.Value() == currentUsername() {
				m.userInput.SetValue(cfg.Server.User)
			}
			m.reconfig = true
		} else if cfg.IsLocal() && len(cfg.BrowseRoots) > 0 {
			m.reconfig = true
		}
	}

	return m
}

func defaultSSHFactory(host, user string) ssh.Runner {
	return ssh.NewClient(host, user)
}

func currentUsername() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// Launch returns true if the wizard completed and the user wants to launch the dashboard.
func (m InitModel) Launch() bool {
	return m.launch
}

func (m InitModel) Init() tea.Cmd {
	if m.step == stepConnect {
		return tea.Batch(
			m.spinner.Tick,
			testSSHCmd(m.host, m.user, m.sshFactory),
		)
	}
	return m.spinner.Tick
}

func (m InitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == keyCtrlC {
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case sshTestedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepHost
			m.hostInput.Focus()
			return m, nil
		}
		m.goarch = msg.goarch
		m.homeDir = msg.homeDir
		m.connectSub = subDeploy
		m.progress = fmt.Sprintf("SSH ok · Uploading binary (%s)...", m.goarch)
		return m, deployCmd(m.host, m.user, m.goarch, m.homeDir, m.sshFactory)

	case deployedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepHost
			m.hostInput.Focus()
			return m, nil
		}
		m.connectSub = subDiscover
		m.progress = "Binary deployed · Detecting sandbox support..."
		return m, tea.Batch(
			detectSandboxCmd(m.host, m.user, m.sshFactory),
			discoverDirsCmd(m.host, m.user, m.homeDir, m.sshFactory),
		)

	case sandboxDetectedMsg:
		m.sandboxOK = msg.supported
		m.sandboxWhy = msg.reason
		return m, nil

	case dirsFoundMsg:
		if msg.err != nil {
			m.err = msg.err
			if m.step != stepDirs {
				m.step = stepHost
				m.hostInput.Focus()
			}
			return m, nil
		}
		var entries []dirBrowserEntry
		for _, d := range msg.dirs {
			entries = append(entries, dirBrowserEntry{name: d.path, count: d.count})
		}
		if m.step != stepDirs {
			// Initial discovery — set up the browser
			m.browser = newDirBrowser(m.homeDir)
			m.browser.SetRootEntries(entries)
		}
		m.browser.SetEntries(entries)
		m.step = stepDirs
		m.err = nil
		return m, nil

	case configWrittenMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepDirs
			return m, nil
		}
		m.err = nil
		// Local mode — no sync needed, files are already local
		if m.localMode {
			m.step = stepDone
			return m, nil
		}
		// Start syncing selected folders
		if len(m.selected) > 0 {
			m.step = stepSyncing
			m.syncTotal = len(m.selected)
			m.syncDone = 0
			// Sync the first folder
			for path := range m.selected {
				m.syncCurrent = path
				return m, setupSyncCmd(m.host, m.user, m.homeDir, path)
			}
		}
		m.step = stepDone
		return m, nil

	case syncSetupMsg:
		m.syncDone++
		if msg.err != nil {
			m.syncErr = msg.err
		}
		// Find next un-synced folder
		done := 0
		for path := range m.selected {
			done++
			if done > m.syncDone {
				m.syncCurrent = path
				return m, setupSyncCmd(m.host, m.user, m.homeDir, path)
			}
		}
		// All done
		m.step = stepDone
		return m, nil
	}

	// Route to step handlers
	switch m.step {
	case stepMode:
		return m.updateMode(msg)
	case stepHost:
		return m.updateHost(msg)
	case stepUser:
		return m.updateUser(msg)
	case stepConnect:
		return m, nil // async — no user input
	case stepDirs:
		return m.updateDirs(msg)
	case stepSettings:
		return m.updateSettings(msg)
	case stepWrite:
		return m, nil // async
	case stepSyncing:
		return m, nil // async
	case stepDone:
		return m.updateDone(msg)
	}

	return m, nil
}

func (m InitModel) updateMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "l":
			m.localMode = true
			home, _ := os.UserHomeDir()
			m.homeDir = home
			m.progress = "Scanning directories..."
			// Stay at stepMode — dirsFoundMsg handler will set up the browser
			// and transition to stepDirs with root entries properly set.
			return m, discoverLocalDirsCmd(home)
		case "r":
			m.step = stepHost
			m.hostInput.Focus()
			return m, nil
		}
	}
	return m, nil
}

func (m InitModel) updateHost(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if kmsg.String() == keyAttach { // enter
			val := strings.TrimSpace(m.hostInput.Value())
			if val == "" {
				return m, nil
			}
			m.host = val
			m.step = stepUser
			m.err = nil
			m.hostInput.Blur()
			m.userInput.Focus()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.hostInput, cmd = m.hostInput.Update(msg)
	return m, cmd
}

func (m InitModel) updateUser(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case keyAttach: // enter
			val := strings.TrimSpace(m.userInput.Value())
			if val == "" {
				return m, nil
			}
			m.user = val
			m.step = stepConnect
			m.connectSub = subSSH
			m.progress = "Testing SSH connection..."
			m.err = nil
			m.userInput.Blur()
			return m, testSSHCmd(m.host, m.user, m.sshFactory)
		case keyEsc:
			m.step = stepHost
			m.userInput.Blur()
			m.hostInput.Focus()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.userInput, cmd = m.userInput.Update(msg)
	return m, cmd
}

func (m InitModel) updateDirs(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Enter = done (requires selections)
	if kmsg, ok := msg.(tea.KeyPressMsg); ok && kmsg.String() == keyAttach {
		if len(m.selected) > 0 {
			m.step = stepSettings
			return m, nil
		}
		// No selections — ignore enter (use → to drill in)
		return m, nil
	}

	action, cmd := m.browser.Update(msg)

	switch action {
	case dirBrowserAtRoot:
		if m.localMode {
			m.localMode = false
			m.step = stepMode
		} else {
			m.step = stepUser
			m.userInput.Focus()
		}
		return m, nil
	case dirBrowserSelected: // space — toggle
		if e, ok := m.browser.SelectedEntry(); ok {
			rel := m.browser.RelPath()
			var full string
			if rel == "" {
				full = e.name
			} else {
				full = rel + "/" + e.name
			}
			if m.selected[full] {
				delete(m.selected, full)
			} else {
				m.selected[full] = true
			}
		}
		return m, nil
	case dirBrowserConfirm: // tab — done
		if len(m.selected) == 0 {
			return m, nil
		}
		m.step = stepSettings
		return m, nil
	case dirBrowserDrillIn:
		if m.localMode {
			return m, discoverLocalDirsCmd(m.browser.absPath)
		}
		return m, discoverDirsCmd(m.host, m.user, m.browser.absPath, m.sshFactory)
	case dirBrowserDrillUp:
		if m.browser.scanning {
			if m.localMode {
				return m, discoverLocalDirsCmd(m.browser.absPath)
			}
			return m, discoverDirsCmd(m.host, m.user, m.browser.absPath, m.sshFactory)
		}
		return m, nil
	}

	return m, cmd
}

func (m InitModel) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "p":
			m.passthrough = !m.passthrough
			return m, nil
		case keyAttach: // enter — save and continue
			m.step = stepWrite
			var roots []string
			for path := range m.selected {
				roots = append(roots, path)
			}
			if m.localMode {
				return m, writeLocalConfigCmd(m.homeDir, roots, m.passthrough)
			}
			return m, writeConfigCmd(m.host, m.user, m.homeDir, roots, m.passthrough, m.sshFactory)
		case keyEsc:
			m.step = stepDirs
			return m, nil
		}
	}
	return m, nil
}

func (m InitModel) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case keyAttach: // enter → launch dashboard
			m.launch = true
			return m, tea.Quit
		case keyQuit:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m InitModel) View() tea.View {
	var b strings.Builder

	title := "  FUSEBOX  ·  Setup"
	if m.reconfig {
		title += " (reconfiguring)"
	}
	b.WriteString(headerStyle.Render(title))
	b.WriteString("\n\n")

	// Step indicators
	if m.localMode && m.step >= stepMode {
		b.WriteString(stepDoneStyle.Render("  ✓ Mode         local"))
		b.WriteString("\n")
	} else if m.step > stepMode {
		b.WriteString(stepDoneStyle.Render("  ✓ Mode         remote"))
		b.WriteString("\n")
		m.renderStepLine(&b, stepHost, "Server", m.host)
		m.renderStepLine(&b, stepUser, "Username", m.user)
		m.renderConnectLine(&b)
	}
	m.renderDirsLine(&b)
	m.renderSettingsLine(&b)
	m.renderWriteLine(&b)
	if !m.localMode {
		m.renderSyncingLine(&b)
	}

	b.WriteString("\n")

	// Active step content
	switch m.step {
	case stepMode:
		if m.localMode {
			// Waiting for local directory scan
			b.WriteString(fmt.Sprintf("  %s %s\n", m.spinner.View(), m.progress))
		} else {
			b.WriteString("  How do you want to run fusebox?\n\n")
			b.WriteString("  [l]  Local — run Claude sessions on this Mac\n")
			b.WriteString("  [r]  Remote — deploy to a server\n")
		}
	case stepHost:
		if m.err != nil {
			b.WriteString(stepErrStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
			b.WriteString("\n\n")
		}
		b.WriteString("  Server: ")
		b.WriteString(m.hostInput.View())
		b.WriteString("\n")
	case stepUser:
		b.WriteString("  Username: ")
		b.WriteString(m.userInput.View())
		b.WriteString("\n")
	case stepConnect:
		// progress shown in step line
	case stepDirs:
		b.WriteString("  What folders do you want to work on?\n")
		if m.localMode {
			b.WriteString(helpStyle.Render("  These will be available as browse roots for Claude sessions."))
		} else {
			b.WriteString(helpStyle.Render("  These will be synced to the server and available for Claude sessions."))
		}
		b.WriteString("\n")
		if path := m.browser.DisplayPath(); path != "" {
			b.WriteString(helpStyle.Render(fmt.Sprintf("  %s", path)))
			b.WriteString("\n")
		}
		b.WriteString(m.browser.ViewFilter())
		b.WriteString("\n")

		checkIndicator := func(e dirBrowserEntry) string {
			rel := m.browser.RelPath()
			var full string
			if rel == "" {
				full = e.name
			} else {
				full = rel + "/" + e.name
			}
			if m.selected[full] {
				return checkOn.String() + " "
			}
			return checkOff.String() + " "
		}
		b.WriteString(m.browser.ViewEntries(checkIndicator))

		b.WriteString("\n")
		help := "  [space] toggle  [→] open  [/] filter  [enter] done"
		if !m.browser.AtRoot() {
			help += "  [←] up"
		} else {
			help += "  [esc] back"
		}
		b.WriteString(helpStyle.Render(help))
		b.WriteString("\n")
	case stepSettings:
		b.WriteString("  Session options:\n\n")
		pt := checkOff.String()
		if m.passthrough {
			pt = checkOn.String()
		}
		b.WriteString(fmt.Sprintf("  %s  Clickable links in tmux  [p]\n", pt))
		b.WriteString(helpStyle.Render("        Enables tmux allow-passthrough so embedded hyperlinks work."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  [enter] save  [esc] back"))
		b.WriteString("\n")
	case stepWrite:
		// progress shown in step line
	case stepSyncing:
		// progress shown in step line
	case stepDone:
		b.WriteString("  Setup complete!\n\n")
		b.WriteString(helpStyle.Render("  [enter] launch dashboard  [q] quit"))
		b.WriteString("\n")
	}

	if m.step != stepDone {
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  [ctrl+c] quit"))
		b.WriteString("\n")
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m InitModel) renderStepLine(b *strings.Builder, step initStep, label, value string) {
	if m.step > step {
		b.WriteString(stepDoneStyle.Render(fmt.Sprintf("  ✓ %-12s %s", label, value)))
		b.WriteString("\n")
	} else if m.step == step {
		b.WriteString(stepActiveStyle.Render(fmt.Sprintf("  ● %-12s", label)))
		b.WriteString("\n")
	} else {
		b.WriteString(stepPendingStyle.Render(fmt.Sprintf("    %-12s", label)))
		b.WriteString("\n")
	}
}

func (m InitModel) renderConnectLine(b *strings.Builder) {
	if m.step > stepConnect {
		info := fmt.Sprintf("%s · deployed", m.goarch)
		if m.sandboxOK {
			info += " · sandbox ready"
		} else if m.sandboxWhy != "" {
			info += " · no sandbox: " + m.sandboxWhy
		}
		b.WriteString(stepDoneStyle.Render(fmt.Sprintf("  ✓ %-12s %s", "Connected", info)))
		b.WriteString("\n")
	} else if m.step == stepConnect {
		b.WriteString(fmt.Sprintf("  %s %-12s %s",
			m.spinner.View(),
			"Connecting",
			m.progress,
		))
		b.WriteString("\n")
	} else {
		b.WriteString(stepPendingStyle.Render(fmt.Sprintf("    %-12s", "Connect")))
		b.WriteString("\n")
	}
}

func (m InitModel) renderDirsLine(b *strings.Builder) {
	label := "Folders"
	if m.step > stepDirs {
		count := len(m.selected)
		b.WriteString(stepDoneStyle.Render(fmt.Sprintf("  ✓ %-12s %d selected", label, count)))
		b.WriteString("\n")
	} else if m.step == stepDirs {
		b.WriteString(stepActiveStyle.Render(fmt.Sprintf("  ● %-12s", label)))
		b.WriteString("\n")
	} else {
		b.WriteString(stepPendingStyle.Render(fmt.Sprintf("    %-12s", label)))
		b.WriteString("\n")
	}
}

func (m InitModel) renderSettingsLine(b *strings.Builder) {
	if m.step > stepSettings {
		info := ""
		if m.passthrough {
			info = "passthrough on"
		}
		if info == "" {
			info = "defaults"
		}
		b.WriteString(stepDoneStyle.Render(fmt.Sprintf("  ✓ %-12s %s", "Options", info)))
		b.WriteString("\n")
	} else if m.step == stepSettings {
		b.WriteString(stepActiveStyle.Render(fmt.Sprintf("  ● %-12s", "Options")))
		b.WriteString("\n")
	} else {
		b.WriteString(stepPendingStyle.Render(fmt.Sprintf("    %-12s", "Options")))
		b.WriteString("\n")
	}
}

func (m InitModel) renderWriteLine(b *strings.Builder) {
	if m.step > stepWrite {
		b.WriteString(stepDoneStyle.Render(fmt.Sprintf("  ✓ %-12s ~/.config/fusebox/config.yaml", "Config")))
		b.WriteString("\n")
	} else if m.step == stepWrite {
		b.WriteString(fmt.Sprintf("  %s %-12s Writing config...",
			m.spinner.View(),
			"Config",
		))
		b.WriteString("\n")
	} else {
		b.WriteString(stepPendingStyle.Render(fmt.Sprintf("    %-12s", "Config")))
		b.WriteString("\n")
	}
}

func (m InitModel) renderSyncingLine(b *strings.Builder) {
	if len(m.selected) == 0 {
		return // nothing to sync, skip this line entirely
	}
	if m.step > stepSyncing {
		info := fmt.Sprintf("%d folders synced", m.syncDone)
		if m.syncErr != nil {
			info += " (with errors)"
		}
		b.WriteString(stepDoneStyle.Render(fmt.Sprintf("  ✓ %-12s %s", "Sync", info)))
		b.WriteString("\n")
	} else if m.step == stepSyncing {
		info := fmt.Sprintf("Syncing %s (%d/%d)...", m.syncCurrent, m.syncDone+1, m.syncTotal)
		b.WriteString(fmt.Sprintf("  %s %-12s %s",
			m.spinner.View(),
			"Sync",
			info,
		))
		b.WriteString("\n")
	} else {
		b.WriteString(stepPendingStyle.Render(fmt.Sprintf("    %-12s", "Sync")))
		b.WriteString("\n")
	}
}
