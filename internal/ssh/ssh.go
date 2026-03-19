package ssh

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes SSH commands against a remote host.
type Runner interface {
	Run(command string) ([]byte, error)
	AttachCmd(session string) *exec.Cmd
	AttachPaneCmd(session string, pane int) *exec.Cmd
}

// Client runs commands over SSH using the ssh binary.
type Client struct {
	Host string
	User string
}

func NewClient(host, user string) *Client {
	return &Client{Host: host, User: user}
}

func (c *Client) target() string {
	return fmt.Sprintf("%s@%s", c.User, c.Host)
}

// Run executes a command on the remote host and returns stdout.
// On failure, stderr from the remote command is included in the error.
func (c *Client) Run(command string) ([]byte, error) {
	cmd := exec.Command("ssh", c.target(), command)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
	}
	return out, err
}

// AttachCmd builds an ssh -t command for attaching to a tmux session.
// The caller is responsible for running this (e.g. via tea.ExecProcess).
func (c *Client) AttachCmd(session string) *exec.Cmd {
	return exec.Command("ssh", "-t", c.target(),
		"tmux", "attach-session", "-t", session)
}

// AttachPaneCmd builds an ssh -t command that selects a specific pane then attaches.
func (c *Client) AttachPaneCmd(session string, pane int) *exec.Cmd {
	cmd := fmt.Sprintf("tmux select-pane -t %s:0.%d && tmux attach-session -t %s", session, pane, session)
	return exec.Command("ssh", "-t", c.target(), "sh", "-c", cmd)
}

// LocalRunner executes commands directly on the local machine (no SSH).
// Used when the TUI is running on the same host as the server.
type LocalRunner struct {
	ServerPath string
}

func NewLocalRunner(serverPath string) *LocalRunner {
	return &LocalRunner{ServerPath: serverPath}
}

// Run executes a command locally via sh -c.
func (l *LocalRunner) Run(command string) ([]byte, error) {
	cmd := exec.Command("sh", "-c", command)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
	}
	return out, err
}

// AttachCmd builds a command to attach to a local tmux session.
func (l *LocalRunner) AttachCmd(session string) *exec.Cmd {
	return exec.Command("tmux", "attach-session", "-t", session)
}

// AttachPaneCmd builds a command to select a pane and attach locally.
func (l *LocalRunner) AttachPaneCmd(session string, pane int) *exec.Cmd {
	cmd := fmt.Sprintf("tmux select-pane -t %s:0.%d && tmux attach-session -t %s", session, pane, session)
	return exec.Command("sh", "-c", cmd)
}
