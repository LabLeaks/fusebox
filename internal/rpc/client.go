package rpc

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	fusesync "github.com/lableaks/fusebox/internal/sync"
)

// ClientConfig configures an RPC client connection.
type ClientConfig struct {
	Address     string // host:port of the local daemon
	Secret      string
	SyncWaiter  fusesync.SyncWaiter // optional: if set, sync-wait before exec
	SessionName string              // Mutagen session name for sync-wait
	SyncTimeout time.Duration       // how long to wait for sync (default 30s)
}

// Client connects to the local daemon and sends RPC requests.
type Client struct {
	cfg     ClientConfig
	conn    net.Conn
	encoder *Encoder
	decoder *Decoder
}

// Dial connects to the daemon at the configured address.
func Dial(cfg ClientConfig) (*Client, error) {
	if cfg.SyncTimeout == 0 {
		cfg.SyncTimeout = 30 * time.Second
	}

	conn, err := net.DialTimeout("tcp", cfg.Address, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("local machine unreachable: %w", err)
	}

	return &Client{
		cfg:     cfg,
		conn:    conn,
		encoder: NewEncoder(conn),
		decoder: NewDecoder(conn),
	}, nil
}

// NewClient creates a client from an existing connection (for testing).
func NewClient(conn io.ReadWriter, cfg ClientConfig) *Client {
	if cfg.SyncTimeout == 0 {
		cfg.SyncTimeout = 30 * time.Second
	}
	return &Client{
		cfg:     cfg,
		encoder: NewEncoder(conn),
		decoder: NewDecoder(conn),
	}
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ExecStream sends an exec request and calls handler for each response message.
// It performs sync-wait before sending if a SyncWaiter is configured.
// The handler receives the message type and raw JSON bytes.
// Returns when an exit or error message is received.
func (c *Client) ExecStream(action string, params map[string]string, log io.Writer, handler StreamHandler) error {
	// Sync-wait before exec if configured
	if c.cfg.SyncWaiter != nil && c.cfg.SessionName != "" {
		if err := fusesync.WaitForSyncWithLog(c.cfg.SyncWaiter, c.cfg.SessionName, c.cfg.SyncTimeout, log); err != nil {
			return fmt.Errorf("sync-wait: %w", err)
		}
	}

	req := ExecRequest{
		Type:   TypeExec,
		Secret: c.cfg.Secret,
		Action: action,
		Params: params,
	}
	if err := c.encoder.Send(req); err != nil {
		return fmt.Errorf("sending exec request: %w", err)
	}

	return c.streamResponses(handler)
}

// RequestActions queries the daemon for available actions.
func (c *Client) RequestActions() (*ActionsResponse, error) {
	req := ActionsRequest{
		Type:   TypeActions,
		Secret: c.cfg.Secret,
	}
	if err := c.encoder.Send(req); err != nil {
		return nil, fmt.Errorf("sending actions request: %w", err)
	}

	msgType, raw, err := c.decoder.ReceiveEnvelope()
	if err != nil {
		return nil, fmt.Errorf("receiving actions response: %w", err)
	}

	switch msgType {
	case TypeActionsResponse:
		var resp ActionsResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("unmarshaling actions response: %w", err)
		}
		return &resp, nil
	case TypeError:
		var errResp ErrorResponse
		if err := json.Unmarshal(raw, &errResp); err != nil {
			return nil, fmt.Errorf("unmarshaling error response: %w", err)
		}
		return nil, fmt.Errorf("RPC error [%s]: %s", errResp.Code, errResp.Message)
	default:
		return nil, fmt.Errorf("unexpected message type %q", msgType)
	}
}

// StreamHandler receives decoded RPC response messages during exec streaming.
type StreamHandler interface {
	OnStdout(line string)
	OnStderr(line string)
	OnExit(code int, durationMs int64)
	OnError(code, message string)
}

// streamResponses reads messages until exit or error.
func (c *Client) streamResponses(handler StreamHandler) error {
	for {
		msgType, raw, err := c.decoder.ReceiveEnvelope()
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("connection closed unexpectedly")
			}
			return fmt.Errorf("receiving response: %w", err)
		}

		switch msgType {
		case TypeStdout:
			var msg StdoutMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				return fmt.Errorf("unmarshaling stdout: %w", err)
			}
			handler.OnStdout(msg.Line)

		case TypeStderr:
			var msg StderrMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				return fmt.Errorf("unmarshaling stderr: %w", err)
			}
			handler.OnStderr(msg.Line)

		case TypeExit:
			var msg ExitMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				return fmt.Errorf("unmarshaling exit: %w", err)
			}
			handler.OnExit(msg.Code, msg.Duration)
			return nil

		case TypeError:
			var msg ErrorResponse
			if err := json.Unmarshal(raw, &msg); err != nil {
				return fmt.Errorf("unmarshaling error: %w", err)
			}
			handler.OnError(msg.Code, msg.Message)
			return fmt.Errorf("RPC error [%s]: %s", msg.Code, msg.Message)

		default:
			return fmt.Errorf("unexpected message type %q", msgType)
		}
	}
}
