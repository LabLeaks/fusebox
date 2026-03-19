package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/lableaks/fusebox/internal/config"
	embedpkg "github.com/lableaks/fusebox/internal/embed"
	"github.com/lableaks/fusebox/internal/ssh"
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
		if _, err := runner.Run("mkdir -p " + binDir + " ~/.config/work-cli"); err != nil {
			return deployedMsg{err: fmt.Errorf("failed to create directories: %w", err)}
		}

		// Write binary to temp file locally, then SCP it
		tmpFile, err := os.CreateTemp("", "work-server-*")
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
		target := fmt.Sprintf("%s@%s:%s/work", user, host, binDir)
		scpCmd := fmt.Sprintf("scp -q %s %s", tmpFile.Name(), target)
		if _, err := runLocalCmd(scpCmd); err != nil {
			return deployedMsg{err: fmt.Errorf("failed to upload binary: %w", err)}
		}

		// Symlink work-helper and run install-hooks + fix-mouse
		commands := fmt.Sprintf(
			"ln -sf %s/work %s/work-helper && %s/work install-hooks && %s/work fix-mouse",
			binDir, binDir, binDir, binDir,
		)
		if _, err := runner.Run(commands); err != nil {
			return deployedMsg{err: fmt.Errorf("failed to configure server: %w", err)}
		}

		return deployedMsg{}
	}
}

// discoverDirsCmd lists non-hidden subdirectories at scanPath (absolute) on the server.
func discoverDirsCmd(host, user, scanPath string, factory func(host, user string) ssh.Runner) tea.Cmd {
	return func() tea.Msg {
		runner := factory(host, user)
		cmd := fmt.Sprintf(
			`for d in %s/*/; do [ -d "$d" ] && name=$(basename "$d") && case "$name" in .*) continue;; esac && count=$(find "$d" -maxdepth 1 -mindepth 1 -type d 2>/dev/null | wc -l) && echo "$name $count"; done`,
			scanPath,
		)
		out, err := runner.Run(cmd)
		if err != nil {
			return dirsFoundMsg{err: fmt.Errorf("failed to list directories: %w", err)}
		}

		var dirs []dirEntry
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, " ", 2)
			name := parts[0]
			count := 0
			if len(parts) == 2 {
				fmt.Sscanf(parts[1], "%d", &count)
			}
			dirs = append(dirs, dirEntry{path: name, count: count})
		}

		return dirsFoundMsg{dirs: dirs}
	}
}

// writeConfigCmd writes the config file and roots.conf on the server.
func writeConfigCmd(host, user, homeDir string, roots []string, factory func(host, user string) ssh.Runner) tea.Cmd {
	return func() tea.Msg {
		// Build browse_roots with ~ prefix
		browseRoots := make([]string, len(roots))
		for i, r := range roots {
			browseRoots[i] = "~/" + r
		}

		cfg := config.DefaultConfig()
		cfg.Server.Host = host
		cfg.Server.User = user
		cfg.Server.HomeDir = homeDir
		cfg.BrowseRoots = browseRoots

		if err := config.Save(cfg); err != nil {
			return configWrittenMsg{err: fmt.Errorf("failed to save config: %w", err)}
		}

		// Write roots.conf on server
		if len(roots) > 0 {
			runner := factory(host, user)
			var serverRoots []string
			for _, r := range roots {
				serverRoots = append(serverRoots, homeDir+"/"+r)
			}
			rootsContent := strings.Join(serverRoots, "\n")
			cmd := fmt.Sprintf("cat > ~/.config/work-cli/roots.conf << 'ROOTSEOF'\n%s\nROOTSEOF", rootsContent)
			if _, err := runner.Run(cmd); err != nil {
				return configWrittenMsg{err: fmt.Errorf("failed to write roots.conf: %w", err)}
			}
		}

		return configWrittenMsg{}
	}
}

// runLocalCmd executes a local shell command.
func runLocalCmd(command string) ([]byte, error) {
	cmd := newExecCommand("sh", "-c", command)
	return cmd.Output()
}
