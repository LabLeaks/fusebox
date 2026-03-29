package sync

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// defaultIgnores are always added to every Mutagen sync session.
var defaultIgnores = []string{".git", "fusebox.yaml"}

// CommandRunner abstracts exec.Command for testing.
type CommandRunner interface {
	Run(name string, args ...string) (stdout string, stderr string, err error)
}

// execRunner runs commands via os/exec.
type execRunner struct{}

func (e *execRunner) Run(name string, args ...string) (string, string, error) {
	cmd := exec.Command(name, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// MutagenManager wraps the Mutagen CLI for managing sync sessions.
type MutagenManager struct {
	mutagenPath string
	runner      CommandRunner
}

// NewMutagenManager creates a MutagenManager after verifying the mutagen binary exists.
func NewMutagenManager() (*MutagenManager, error) {
	path, err := exec.LookPath("mutagen")
	if err != nil {
		return nil, fmt.Errorf("mutagen binary not found on PATH: install with 'brew install mutagen': %w", err)
	}
	return &MutagenManager{
		mutagenPath: path,
		runner:      &execRunner{},
	}, nil
}

// NewMutagenManagerWithRunner creates a MutagenManager with a custom command runner (for testing).
func NewMutagenManagerWithRunner(runner CommandRunner) *MutagenManager {
	return &MutagenManager{
		mutagenPath: "mutagen",
		runner:      runner,
	}
}

// SrcSessionName returns the session name for source sync.
func SrcSessionName(project string) string {
	return "fusebox-src-" + project
}

// ClaudeSessionName returns the session name for Claude state sync.
func ClaudeSessionName(project string) string {
	return "fusebox-claude-" + project
}

// mergeIgnores combines user ignores with defaults, deduplicating entries.
func mergeIgnores(userIgnores []string) []string {
	seen := make(map[string]struct{})
	var result []string

	for _, ig := range defaultIgnores {
		if _, ok := seen[ig]; !ok {
			seen[ig] = struct{}{}
			result = append(result, ig)
		}
	}
	for _, ig := range userIgnores {
		if _, ok := seen[ig]; !ok {
			seen[ig] = struct{}{}
			result = append(result, ig)
		}
	}
	return result
}

// buildCreateArgs constructs the argument list for `mutagen sync create`.
func buildCreateArgs(sessionName, localPath, remoteUser, remoteHost, remotePath string, ignores []string) []string {
	merged := mergeIgnores(ignores)

	alpha := localPath
	beta := fmt.Sprintf("%s@%s:%s", remoteUser, remoteHost, remotePath)

	args := []string{"sync", "create", alpha, beta, "--name", sessionName}
	for _, ig := range merged {
		args = append(args, "--ignore", ig)
	}
	return args
}

// Create starts a new Mutagen sync session.
func (m *MutagenManager) Create(sessionName, localPath, remoteUser, remoteHost, remotePath string, ignores []string) error {
	args := buildCreateArgs(sessionName, localPath, remoteUser, remoteHost, remotePath, ignores)
	_, stderr, err := m.runner.Run(m.mutagenPath, args...)
	if err != nil {
		return fmt.Errorf("mutagen sync create: %s: %w", strings.TrimSpace(stderr), err)
	}
	return nil
}

// Resume resumes a paused Mutagen sync session.
func (m *MutagenManager) Resume(sessionName string) error {
	_, stderr, err := m.runner.Run(m.mutagenPath, "sync", "resume", sessionName)
	if err != nil {
		return fmt.Errorf("mutagen sync resume %s: %s: %w", sessionName, strings.TrimSpace(stderr), err)
	}
	return nil
}

// Pause pauses an active Mutagen sync session.
func (m *MutagenManager) Pause(sessionName string) error {
	_, stderr, err := m.runner.Run(m.mutagenPath, "sync", "pause", sessionName)
	if err != nil {
		return fmt.Errorf("mutagen sync pause %s: %s: %w", sessionName, strings.TrimSpace(stderr), err)
	}
	return nil
}

// Terminate terminates a Mutagen sync session.
func (m *MutagenManager) Terminate(sessionName string) error {
	_, stderr, err := m.runner.Run(m.mutagenPath, "sync", "terminate", sessionName)
	if err != nil {
		return fmt.Errorf("mutagen sync terminate %s: %s: %w", sessionName, strings.TrimSpace(stderr), err)
	}
	return nil
}

// SessionStatus returns the status string for a named session.
// It parses the output of `mutagen sync list <name>`.
func (m *MutagenManager) SessionStatus(sessionName string) (string, error) {
	stdout, stderr, err := m.runner.Run(m.mutagenPath, "sync", "list", sessionName)
	if err != nil {
		return "", fmt.Errorf("mutagen sync list %s: %s: %w", sessionName, strings.TrimSpace(stderr), err)
	}
	return parseStatus(stdout), nil
}

// parseStatus extracts the Status field from mutagen sync list output.
func parseStatus(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Status:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "Status:"))
		}
	}
	return "Unknown"
}

// WaitForSync polls SessionStatus until the session reaches "Watching for changes"
// or the timeout expires.
func (m *MutagenManager) WaitForSync(sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	poll := 500 * time.Millisecond

	for {
		status, err := m.SessionStatus(sessionName)
		if err != nil {
			return fmt.Errorf("polling sync status: %w", err)
		}
		if status == "Watching for changes" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("sync timeout after %v: last status %q", timeout, status)
		}
		time.Sleep(poll)
	}
}
