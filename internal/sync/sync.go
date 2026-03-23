package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultIgnores lists patterns mutagen should skip when syncing.
var defaultIgnores = []string{
	"node_modules",
	"vendor",
	"__pycache__",
	".venv",
	"venv",
	".next",
	".cache",
	".DS_Store",
}

// Manager wraps the mutagen CLI for bidirectional file sync.
type Manager struct {
	DataDir   string // ~/.fusebox
	SSHTarget string // user@host
}

// SyncSession represents an active mutagen sync session.
type SyncSession struct {
	Name   string // derived from local path
	Local  string // local path
	Remote string // server path (~/.fusebox/sync/<name>)
	Status string // Watching for changes, Staging files, etc.
}

func NewManager(dataDir, sshTarget string) *Manager {
	return &Manager{DataDir: dataDir, SSHTarget: sshTarget}
}

func (m *Manager) mutagenBin() string {
	custom := filepath.Join(m.DataDir, "bin", "mutagen")
	if _, err := os.Stat(custom); err == nil {
		return custom
	}
	if path, err := exec.LookPath("mutagen"); err == nil {
		return path
	}
	return custom // will fail with a clear error
}

func (m *Manager) syncName(localPath string) string {
	return "fusebox-" + filepath.Base(localPath)
}

func (m *Manager) remotePath(localPath string) string {
	return fmt.Sprintf("%s:~/.fusebox/sync/%s", m.SSHTarget, filepath.Base(localPath))
}

// Add starts syncing a local folder to the server.
func (m *Manager) Add(localPath string) error {
	if err := m.EnsureMutagen(); err != nil {
		return err
	}

	abs, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Guard against nested syncs
	if err := m.checkNested(abs); err != nil {
		return err
	}

	// Ensure the remote sync directory exists
	name := filepath.Base(abs)
	mkdirCmd := exec.Command("ssh", m.SSHTarget, "mkdir", "-p", "~/.fusebox/sync/"+name)
	if out, err := mkdirCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create remote dir: %s: %w", strings.TrimSpace(string(out)), err)
	}

	bin := m.mutagenBin()
	args := []string{"sync", "create",
		abs, m.remotePath(abs),
		"--name", m.syncName(abs),
		"--label", "fusebox=true",
		"--sync-mode", "two-way-resolved",
	}
	// Default ignores — skip build artifacts and dependency dirs
	for _, pattern := range defaultIgnores {
		args = append(args, "--ignore", pattern)
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mutagen sync create: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Remove stops syncing a local folder.
func (m *Manager) Remove(localPath string) error {
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	bin := m.mutagenBin()
	cmd := exec.Command(bin, "sync", "terminate", m.syncName(abs))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mutagen sync terminate: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// List returns all fusebox-managed sync sessions.
func (m *Manager) List() ([]SyncSession, error) {
	bin := m.mutagenBin()
	if _, err := os.Stat(bin); err != nil {
		if p, err2 := exec.LookPath("mutagen"); err2 != nil {
			return nil, nil // mutagen not installed, no sessions
		} else {
			bin = p
		}
	}

	cmd := exec.Command(bin, "sync", "list", "--label-selector", "fusebox=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(outStr), "no session") || outStr == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("mutagen sync list: %s", outStr)
	}
	return parseMutagenList(string(out)), nil
}

// Pause pauses all fusebox sync sessions.
func (m *Manager) Pause() error {
	bin := m.mutagenBin()
	cmd := exec.Command(bin, "sync", "pause", "--label-selector", "fusebox=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mutagen sync pause: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Resume resumes all fusebox sync sessions.
func (m *Manager) Resume() error {
	bin := m.mutagenBin()
	cmd := exec.Command(bin, "sync", "resume", "--label-selector", "fusebox=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mutagen sync resume: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// EnsureMutagen installs mutagen if not found in PATH or DataDir.
func (m *Manager) EnsureMutagen() error {
	if _, err := exec.LookPath("mutagen"); err == nil {
		return nil
	}
	custom := filepath.Join(m.DataDir, "bin", "mutagen")
	if _, err := os.Stat(custom); err == nil {
		return nil
	}
	return installMutagen(filepath.Join(m.DataDir, "bin"))
}

// checkNested returns an error if the path overlaps with an existing sync.
func (m *Manager) checkNested(abs string) error {
	existing, err := m.List()
	if err != nil {
		return nil // can't check, allow it
	}
	return checkNestedAgainst(existing, abs)
}

// checkNestedAgainst checks a path against a list of existing syncs.
func checkNestedAgainst(existing []SyncSession, abs string) error {
	absSlash := abs + "/"
	for _, s := range existing {
		other := s.Local
		if other == "" {
			continue
		}
		otherSlash := other + "/"

		if abs == other {
			return fmt.Errorf("already syncing %s", other)
		}
		if strings.HasPrefix(absSlash, otherSlash) {
			return fmt.Errorf("%s is inside already-synced %s", abs, other)
		}
		if strings.HasPrefix(otherSlash, absSlash) {
			return fmt.Errorf("%s contains already-synced %s", abs, other)
		}
	}
	return nil
}

// parseMutagenList parses `mutagen sync list` text output into sessions.
func parseMutagenList(output string) []SyncSession {
	var sessions []SyncSession
	var current *SyncSession
	section := ""

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "Name:") {
			if current != nil {
				sessions = append(sessions, *current)
			}
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "Name:"))
			current = &SyncSession{Name: name}
			section = ""
			continue
		}

		if current == nil {
			continue
		}

		switch trimmed {
		case "Alpha:":
			section = "alpha"
			continue
		case "Beta:":
			section = "beta"
			continue
		}

		if strings.HasPrefix(trimmed, "URL:") {
			url := strings.TrimSpace(strings.TrimPrefix(trimmed, "URL:"))
			switch section {
			case "alpha":
				current.Local = url
			case "beta":
				current.Remote = url
			}
			continue
		}

		if strings.HasPrefix(trimmed, "Status:") {
			current.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, "Status:"))
			continue
		}
	}

	if current != nil {
		sessions = append(sessions, *current)
	}

	return sessions
}
