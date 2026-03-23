package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/lableaks/fusebox/internal/config"
	embedpkg "github.com/lableaks/fusebox/internal/embed"
	"github.com/lableaks/fusebox/internal/ssh"
	syncpkg "github.com/lableaks/fusebox/internal/sync"
)

// Messages for init wizard async operations.

type sshTestedMsg struct {
	goarch  string
	homeDir string
	err     error
}

type deployedMsg struct {
	err error
}

type dirsFoundMsg struct {
	dirs []dirEntry
	err  error
}

type configWrittenMsg struct {
	err error
}

type sandboxDetectedMsg struct {
	supported bool
	reason    string
}

type syncSetupMsg struct {
	path string
	err  error
}

type dirEntry struct {
	path  string
	count int
}

// MapArch converts uname -m output to GOARCH values.
func MapArch(uname string) string {
	uname = strings.TrimSpace(uname)
	switch uname {
	case "aarch64", "arm64":
		return "arm64"
	case "x86_64":
		return "amd64"
	default:
		return uname
	}
}

// testSSHCmd tests SSH connectivity, detects server arch and home dir.
func testSSHCmd(host, user string, factory func(host, user string) ssh.Runner) tea.Cmd {
	return func() tea.Msg {
		runner := factory(host, user)
		out, err := runner.Run("uname -m && echo $HOME")
		if err != nil {
			return sshTestedMsg{err: fmt.Errorf("SSH connection failed: %w", err)}
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) < 2 {
			return sshTestedMsg{err: fmt.Errorf("unexpected server response: %s", string(out))}
		}
		goarch := MapArch(lines[0])
		homeDir := strings.TrimSpace(lines[1])
		return sshTestedMsg{goarch: goarch, homeDir: homeDir}
	}
}

// deployCmd extracts the embedded server binary and deploys it via SSH.
func deployCmd(host, user, goarch, homeDir string, factory func(host, user string) ssh.Runner) tea.Cmd {
	return func() tea.Msg {
		binary, err := embedpkg.ServerBinary(goarch)
		if err != nil {
			return deployedMsg{err: err}
		}

		runner := factory(host, user)

		// Create bin directory
		binDir := homeDir + "/bin"
		if _, err := runner.Run("mkdir -p " + binDir + " ~/.config/fusebox"); err != nil {
			return deployedMsg{err: fmt.Errorf("failed to create directories: %w", err)}
		}

		// Write binary to temp file locally, then SCP it
		tmpFile, err := os.CreateTemp("", "fusebox-server-*")
		if err != nil {
			return deployedMsg{err: fmt.Errorf("failed to create temp file: %w", err)}
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(binary); err != nil {
			tmpFile.Close()
			return deployedMsg{err: fmt.Errorf("failed to write temp file: %w", err)}
		}
		tmpFile.Close()
		if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
			return deployedMsg{err: fmt.Errorf("failed to chmod temp file: %w", err)}
		}

		// SCP the binary
		target := fmt.Sprintf("%s@%s:%s/fusebox", user, host, binDir)
		scpCmd := fmt.Sprintf("scp -q %s %s", tmpFile.Name(), target)
		if _, err := runLocalCmd(scpCmd); err != nil {
			return deployedMsg{err: fmt.Errorf("failed to upload binary: %w", err)}
		}

		// Run install-hooks + fix-mouse
		commands := fmt.Sprintf(
			"%s/fusebox install-hooks && %s/fusebox fix-mouse",
			binDir, binDir,
		)
		if _, err := runner.Run(commands); err != nil {
			return deployedMsg{err: fmt.Errorf("failed to configure server: %w", err)}
		}

		// Ensure ~/.local/bin is in PATH for non-login shells (tmux)
		ensurePathCmd := `grep -q 'HOME/.local/bin' ~/.bashrc 2>/dev/null || sed -i '1i # Added by fusebox — ensures ~/.local/bin is in PATH for tmux/non-login shells\nexport PATH="$HOME/.local/bin:$PATH"\n' ~/.bashrc`
		runner.Run(ensurePathCmd) // best-effort, don't fail deploy

		return deployedMsg{}
	}
}

// detectSandboxCmd checks if the server supports namespace isolation.
func detectSandboxCmd(host, user string, factory func(host, user string) ssh.Runner) tea.Cmd {
	return func() tea.Msg {
		runner := factory(host, user)
		// Check kernel version and user namespace support
		out, err := runner.Run("uname -r && cat /proc/sys/kernel/unprivileged_userns_clone 2>/dev/null || echo ok && cat /proc/sys/user/max_user_namespaces 2>/dev/null || echo ok")
		if err != nil {
			return sandboxDetectedMsg{supported: false, reason: "cannot detect kernel capabilities"}
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) < 1 {
			return sandboxDetectedMsg{supported: false, reason: "unexpected response"}
		}

		// Parse kernel version
		release := strings.TrimSpace(lines[0])
		parts := strings.SplitN(release, ".", 3)
		major, minor := 0, 0
		if len(parts) >= 1 {
			fmt.Sscanf(parts[0], "%d", &major)
		}
		if len(parts) >= 2 {
			minorStr := strings.SplitN(parts[1], "-", 2)[0]
			fmt.Sscanf(minorStr, "%d", &minor)
		}

		if major < 5 || (major == 5 && minor < 11) {
			return sandboxDetectedMsg{supported: false, reason: fmt.Sprintf("kernel %s (need ≥5.11)", release)}
		}

		// Check user namespaces
		if len(lines) > 1 && strings.TrimSpace(lines[1]) == "0" {
			return sandboxDetectedMsg{supported: false, reason: "user namespaces disabled"}
		}
		if len(lines) > 2 && strings.TrimSpace(lines[2]) == "0" {
			return sandboxDetectedMsg{supported: false, reason: "max_user_namespaces=0"}
		}

		return sandboxDetectedMsg{supported: true}
	}
}

// writeConfigCmd writes the config file and roots.conf on the server.
// roots are relative paths from the local home (e.g. "work/lableaks").
func writeConfigCmd(host, user, homeDir string, roots []string, passthrough bool, factory func(host, user string) ssh.Runner) tea.Cmd {
	return func() tea.Msg {
		// Browse roots use ~ prefix (local paths for fusebox sync add)
		browseRoots := make([]string, len(roots))
		for i, r := range roots {
			browseRoots[i] = "~/" + r
		}

		cfg := config.DefaultConfig()
		cfg.Server.Host = host
		cfg.Server.User = user
		cfg.Server.HomeDir = homeDir
		cfg.BrowseRoots = browseRoots
		cfg.Tmux.Passthrough = passthrough

		if err := config.Save(cfg); err != nil {
			return configWrittenMsg{err: fmt.Errorf("failed to save config: %w", err)}
		}

		runner := factory(host, user)

		// Write roots.conf on server — point to sync destinations
		// Synced folders land at ~/.fusebox/sync/<basename>/
		if len(roots) > 0 {
			var serverRoots []string
			for _, r := range roots {
				base := filepath.Base(r)
				serverRoots = append(serverRoots, homeDir+"/.fusebox/sync/"+base)
			}
			rootsContent := strings.Join(serverRoots, "\n")
			cmd := fmt.Sprintf("cat > ~/.config/fusebox/roots.conf << 'ROOTSEOF'\n%s\nROOTSEOF", rootsContent)
			if _, err := runner.Run(cmd); err != nil {
				return configWrittenMsg{err: fmt.Errorf("failed to write roots.conf: %w", err)}
			}
		}

		// Apply tmux passthrough setting on server
		if passthrough {
			runner.Run("tmux set -g allow-passthrough on 2>/dev/null; grep -q allow-passthrough ~/.tmux.conf 2>/dev/null || echo 'set -g allow-passthrough on' >> ~/.tmux.conf")
		}

		// Ensure sandbox data directory exists on server
		runner.Run("mkdir -p ~/.fusebox")

		return configWrittenMsg{}
	}
}

// setupSyncCmd starts a mutagen sync session for a folder.
// path is relative to homeDir (e.g. "work/lableaks").
func setupSyncCmd(host, user, homeDir, relPath string) tea.Cmd {
	return func() tea.Msg {
		localHome, err := os.UserHomeDir()
		if err != nil {
			return syncSetupMsg{path: relPath, err: err}
		}

		localPath := localHome + "/" + relPath
		sshTarget := user + "@" + host
		dataDir := localHome + "/.fusebox"

		mgr := syncpkg.NewManager(dataDir, sshTarget)
		if err := mgr.Add(localPath); err != nil {
			return syncSetupMsg{path: relPath, err: err}
		}
		return syncSetupMsg{path: relPath}
	}
}

// runLocalCmd executes a local shell command.
func runLocalCmd(command string) ([]byte, error) {
	cmd := newExecCommand("sh", "-c", command)
	return cmd.Output()
}

// discoverLocalDirsCmd lists non-hidden subdirectories at scanPath on the local machine.
func discoverLocalDirsCmd(scanPath string) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(scanPath)
		if err != nil {
			return dirsFoundMsg{err: fmt.Errorf("failed to list directories: %w", err)}
		}

		var dirs []dirEntry
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			// Count subdirectories
			count := 0
			subPath := filepath.Join(scanPath, e.Name())
			if subs, err := os.ReadDir(subPath); err == nil {
				for _, s := range subs {
					if s.IsDir() {
						count++
					}
				}
			}
			dirs = append(dirs, dirEntry{path: e.Name(), count: count})
		}

		return dirsFoundMsg{dirs: dirs}
	}
}

// writeLocalConfigCmd writes config for local mode (no server, no sync).
func writeLocalConfigCmd(homeDir string, roots []string, passthrough bool) tea.Cmd {
	return func() tea.Msg {
		// Build browse_roots with absolute paths
		browseRoots := make([]string, len(roots))
		for i, r := range roots {
			browseRoots[i] = filepath.Join(homeDir, r)
		}

		cfg := config.DefaultConfig()
		cfg.BrowseRoots = browseRoots
		cfg.Tmux.Passthrough = passthrough

		if err := config.Save(cfg); err != nil {
			return configWrittenMsg{err: fmt.Errorf("failed to save config: %w", err)}
		}

		// Write roots.conf locally for fusebox dirs command
		confDir := filepath.Join(homeDir, ".config", "fusebox")
		os.MkdirAll(confDir, 0755)
		rootsContent := strings.Join(browseRoots, "\n")
		if err := os.WriteFile(filepath.Join(confDir, "roots.conf"), []byte(rootsContent), 0644); err != nil {
			return configWrittenMsg{err: fmt.Errorf("failed to write roots.conf: %w", err)}
		}

		return configWrittenMsg{}
	}
}
