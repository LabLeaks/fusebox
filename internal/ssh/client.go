package ssh

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Client wraps an SSH connection to a remote server.
type Client struct {
	client *ssh.Client
	host   string
	user   string
	port   int
}

// ConnectOption configures a Client connection.
type ConnectOption func(*connectConfig)

type connectConfig struct {
	port    int
	timeout time.Duration
}

// WithPort sets the SSH port (default 22).
func WithPort(port int) ConnectOption {
	return func(c *connectConfig) {
		c.port = port
	}
}

// WithTimeout sets the connection timeout (default 10s).
func WithTimeout(d time.Duration) ConnectOption {
	return func(c *connectConfig) {
		c.timeout = d
	}
}

// Connect establishes an SSH connection using the system SSH agent for authentication.
func Connect(host, user string, opts ...ConnectOption) (*Client, error) {
	cfg := &connectConfig{
		port:    22,
		timeout: 10 * time.Second,
	}
	for _, o := range opts {
		o(cfg)
	}

	authSock := os.Getenv("SSH_AUTH_SOCK")
	if authSock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set: ssh-agent required")
	}

	agentConn, err := net.Dial("unix", authSock)
	if err != nil {
		return nil, fmt.Errorf("connecting to ssh-agent: %w", err)
	}

	agentClient := agent.NewClient(agentConn)

	sshCfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(agentClient.Signers),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.timeout,
	}

	addr := fmt.Sprintf("%s:%d", host, cfg.port)
	sshClient, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		agentConn.Close()
		return nil, fmt.Errorf("dialing %s: %w", addr, err)
	}

	return &Client{
		client: sshClient,
		host:   host,
		user:   user,
		port:   cfg.port,
	}, nil
}

// Close closes the SSH connection.
func (c *Client) Close() error {
	return c.client.Close()
}

// RunCommand runs a command and returns stdout, stderr, and exit code.
func (c *Client) RunCommand(cmd string) (stdout string, stderr string, exitCode int, err error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", "", -1, fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	runErr := session.Run(cmd)
	if runErr != nil {
		if exitErr, ok := runErr.(*ssh.ExitError); ok {
			return outBuf.String(), errBuf.String(), exitErr.ExitStatus(), nil
		}
		return outBuf.String(), errBuf.String(), -1, fmt.Errorf("running command: %w", runErr)
	}

	return outBuf.String(), errBuf.String(), 0, nil
}

// RunCommandStream runs a command, streaming stdout/stderr to the provided writers.
func (c *Client) RunCommandStream(cmd string, stdout, stderr io.Writer) (exitCode int, err error) {
	session, err := c.client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	runErr := session.Run(cmd)
	if runErr != nil {
		if exitErr, ok := runErr.(*ssh.ExitError); ok {
			return exitErr.ExitStatus(), nil
		}
		return -1, fmt.Errorf("running command: %w", runErr)
	}

	return 0, nil
}

// CopyFile uploads a local file to a remote path using SCP protocol.
func (c *Client) CopyFile(localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("statting local file: %w", err)
	}

	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	pipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		defer pipe.Close()
		// SCP protocol: send file header then content then null byte
		_, err := fmt.Fprintf(pipe, "C0644 %d %s\n", stat.Size(), stat.Name())
		if err != nil {
			errCh <- fmt.Errorf("writing scp header: %w", err)
			return
		}
		if _, err := io.Copy(pipe, f); err != nil {
			errCh <- fmt.Errorf("writing file content: %w", err)
			return
		}
		if _, err := pipe.Write([]byte{0}); err != nil {
			errCh <- fmt.Errorf("writing scp terminator: %w", err)
			return
		}
		errCh <- nil
	}()

	if err := session.Run(fmt.Sprintf("scp -t %s", remotePath)); err != nil {
		return fmt.Errorf("remote scp: %w", err)
	}

	if err := <-errCh; err != nil {
		return err
	}

	return nil
}

// Underlying returns the raw ssh.Client for advanced use cases.
func (c *Client) Underlying() *ssh.Client {
	return c.client
}
