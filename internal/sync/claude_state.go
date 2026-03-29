package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SSHClient is the subset of ssh.Client used for copying global state.
type SSHClient interface {
	CopyFile(localPath, remotePath string) error
	RunCommand(cmd string) (stdout string, stderr string, exitCode int, err error)
}

// EncodePath converts an absolute path to Claude Code's path-encoded format:
// slashes become dashes (e.g., /Users/gk/work/project → -Users-gk-work-project).
func EncodePath(absPath string) string {
	return strings.ReplaceAll(absPath, "/", "-")
}

// CreateClaudeStateSync creates a Mutagen session for syncing Claude Code project state.
// It syncs ~/.claude/projects/<local-path-encoded>/ to the remote equivalent.
func CreateClaudeStateSync(manager *MutagenManager, projectName, localProjectPath, remoteUser, remoteHost, remoteProjectPath string) error {
	localEncoded := EncodePath(localProjectPath)
	remoteEncoded := EncodePath(remoteProjectPath)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	localSyncPath := filepath.Join(homeDir, ".claude", "projects", localEncoded)
	remoteSyncPath := fmt.Sprintf("%s/.claude/projects/%s", remoteHomeDir(remoteUser), remoteEncoded)

	sessionName := ClaudeSessionName(projectName)

	return manager.Create(sessionName, localSyncPath, remoteUser, remoteHost, remoteSyncPath, nil)
}

// remoteHomeDir returns the home directory for a remote user.
// Root's home is /root; all other users use /home/<user>.
func remoteHomeDir(user string) string {
	if user == "root" {
		return "/root"
	}
	return fmt.Sprintf("/home/%s", user)
}

// settingsKeep lists the keys preserved in settings.json during transformation.
var settingsKeep = map[string]bool{
	"alwaysThinkingEnabled":           true,
	"permissions":                     true,
	"env":                             true,
	"skipDangerousModePermissionPrompt": true,
}

// TransformSettings reads a settings.json byte slice and returns a transformed
// version containing only the allowed keys. Unknown keys are dropped.
func TransformSettings(input []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input, &raw); err != nil {
		return nil, fmt.Errorf("parsing settings.json: %w", err)
	}

	filtered := make(map[string]json.RawMessage)
	for key, val := range raw {
		if settingsKeep[key] {
			filtered[key] = val
		}
	}

	out, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling transformed settings: %w", err)
	}
	return out, nil
}

// CopyGlobalState copies Claude Code global config files to the remote host.
// It transforms settings.json (dropping hooks, statusLine, etc.) and copies
// CLAUDE.md, agents/, and skills/ directories as-is.
func CopyGlobalState(sshClient SSHClient, remoteUser string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	localClaudeDir := filepath.Join(homeDir, ".claude")
	remoteClaudeDir := fmt.Sprintf("%s/.claude", remoteHomeDir(remoteUser))

	// Ensure remote .claude directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p %s/agents %s/skills", remoteClaudeDir, remoteClaudeDir)
	_, stderr, exitCode, err := sshClient.RunCommand(mkdirCmd)
	if err != nil {
		return fmt.Errorf("creating remote .claude directories: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("creating remote .claude directories: %s", strings.TrimSpace(stderr))
	}

	// Copy CLAUDE.md
	claudeMD := filepath.Join(localClaudeDir, "CLAUDE.md")
	if _, err := os.Stat(claudeMD); err == nil {
		if err := sshClient.CopyFile(claudeMD, remoteClaudeDir+"/CLAUDE.md"); err != nil {
			return fmt.Errorf("copying CLAUDE.md: %w", err)
		}
	}

	// Transform and copy settings.json
	settingsPath := filepath.Join(localClaudeDir, "settings.json")
	if settingsData, err := os.ReadFile(settingsPath); err == nil {
		transformed, err := TransformSettings(settingsData)
		if err != nil {
			return fmt.Errorf("transforming settings.json: %w", err)
		}

		tmpFile, err := os.CreateTemp("", "fusebox-settings-*.json")
		if err != nil {
			return fmt.Errorf("creating temp file for settings: %w", err)
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.Write(transformed); err != nil {
			tmpFile.Close()
			return fmt.Errorf("writing transformed settings: %w", err)
		}
		tmpFile.Close()

		if err := sshClient.CopyFile(tmpPath, remoteClaudeDir+"/settings.json"); err != nil {
			return fmt.Errorf("copying settings.json: %w", err)
		}
	}

	// Copy agents/ directory contents
	if err := copyDir(sshClient, filepath.Join(localClaudeDir, "agents"), remoteClaudeDir+"/agents"); err != nil {
		return fmt.Errorf("copying agents/: %w", err)
	}

	// Copy skills/ directory contents
	if err := copyDir(sshClient, filepath.Join(localClaudeDir, "skills"), remoteClaudeDir+"/skills"); err != nil {
		return fmt.Errorf("copying skills/: %w", err)
	}

	return nil
}

// copyDir recursively copies all files from a local directory to a remote directory.
// Skips silently if the local directory doesn't exist.
func copyDir(sshClient SSHClient, localDir, remoteDir string) error {
	entries, err := os.ReadDir(localDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading directory %s: %w", localDir, err)
	}

	for _, entry := range entries {
		localPath := filepath.Join(localDir, entry.Name())
		remotePath := remoteDir + "/" + entry.Name()

		if entry.IsDir() {
			mkdirCmd := fmt.Sprintf("mkdir -p %s", remotePath)
			_, stderr, exitCode, err := sshClient.RunCommand(mkdirCmd)
			if err != nil {
				return fmt.Errorf("creating remote directory %s: %w", entry.Name(), err)
			}
			if exitCode != 0 {
				return fmt.Errorf("creating remote directory %s: %s", entry.Name(), strings.TrimSpace(stderr))
			}
			if err := copyDir(sshClient, localPath, remotePath); err != nil {
				return err
			}
			continue
		}

		if err := sshClient.CopyFile(localPath, remotePath); err != nil {
			return fmt.Errorf("copying %s: %w", entry.Name(), err)
		}
	}
	return nil
}
