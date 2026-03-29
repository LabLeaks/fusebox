package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/rpc"
	fusesync "github.com/lableaks/fusebox/internal/sync"
	"github.com/lableaks/fusebox/internal/validation"
)

// Server is the local RPC daemon that executes whitelisted actions.
type Server struct {
	listener    net.Listener
	config      *config.ProjectConfig
	secret      string
	projectDir  string
	logger      *log.Logger
	done        chan struct{}
	wg          sync.WaitGroup
	syncWaiter  fusesync.SyncWaiter
	sessionName string
	syncTimeout time.Duration

	mu         sync.Mutex
	lastAction *LastAction
}

// LastAction records the most recent RPC action for status reporting.
type LastAction struct {
	Name      string    `json:"name"`
	ExitCode  int       `json:"exit_code"`
	Duration  int64     `json:"duration_ms"`
	Timestamp time.Time `json:"timestamp"`
}

// ServerConfig configures a new Server.
type ServerConfig struct {
	Config      *config.ProjectConfig
	Secret      string
	ProjectDir  string
	Logger      *log.Logger
	SyncWaiter  fusesync.SyncWaiter // optional: sync-wait before exec
	SessionName string              // Mutagen session name for sync-wait
	SyncTimeout time.Duration       // how long to wait for sync (default 30s)
}

// NewServer creates a Server bound to the given listener.
func NewServer(listener net.Listener, cfg ServerConfig) *Server {
	syncTimeout := cfg.SyncTimeout
	if syncTimeout == 0 {
		syncTimeout = 30 * time.Second
	}
	return &Server{
		listener:    listener,
		config:      cfg.Config,
		secret:      cfg.Secret,
		projectDir:  cfg.ProjectDir,
		logger:      cfg.Logger,
		syncWaiter:  cfg.SyncWaiter,
		sessionName: cfg.SessionName,
		syncTimeout: syncTimeout,
		done:        make(chan struct{}),
	}
}

// Serve accepts connections until the server is closed.
func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
				s.logger.Printf("accept error: %v", err)
				continue
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// Close stops the server and waits for active connections to finish.
func (s *Server) Close() error {
	close(s.done)
	err := s.listener.Close()
	s.wg.Wait()
	return err
}

// GetLastAction returns the most recent action result.
func (s *Server) GetLastAction() *LastAction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastAction
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	dec := rpc.NewDecoder(conn)
	enc := rpc.NewEncoder(conn)

	for {
		msgType, raw, err := dec.ReceiveEnvelope()
		if err != nil {
			if err == io.EOF {
				return
			}
			s.logger.Printf("read error: %v", err)
			return
		}

		if !s.handleMessage(msgType, raw, enc) {
			return
		}
	}
}

// handleMessage dispatches a single message. Returns false if the connection should close.
func (s *Server) handleMessage(msgType rpc.MessageType, raw []byte, enc *rpc.Encoder) bool {
	switch msgType {
	case rpc.TypeExec:
		var req rpc.ExecRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			s.sendError(enc, "", "PARSE_ERROR", "invalid exec request")
			return false
		}
		if !rpc.ValidateSecret(req.Secret, s.secret) {
			s.sendError(enc, req.Secret, "AUTH_ERROR", "invalid secret")
			return false
		}
		s.handleExec(req, enc)
		return true

	case rpc.TypeActions:
		var req rpc.ActionsRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			s.sendError(enc, "", "PARSE_ERROR", "invalid actions request")
			return false
		}
		if !rpc.ValidateSecret(req.Secret, s.secret) {
			s.sendError(enc, req.Secret, "AUTH_ERROR", "invalid secret")
			return false
		}
		s.handleActions(req, enc)
		return true

	default:
		s.sendError(enc, "", "UNKNOWN_TYPE", fmt.Sprintf("unknown message type: %s", msgType))
		return false
	}
}

func (s *Server) handleExec(req rpc.ExecRequest, enc *rpc.Encoder) {
	action, ok := s.config.Actions[req.Action]
	if !ok {
		s.sendError(enc, req.Secret, "INVALID_ACTION", fmt.Sprintf("unknown action: %s", req.Action))
		return
	}

	s.logger.Printf("[%s] rpc: %s started", time.Now().Format("15:04:05"), req.Action)

	// Validate params
	if err := validation.ValidateParams(req.Params, action.Params); err != nil {
		s.sendError(enc, req.Secret, "INVALID_PARAMS", err.Error())
		s.logger.Printf("[%s] rpc: %s failed (invalid params)", time.Now().Format("15:04:05"), req.Action)
		return
	}

	// Expand template
	expanded, err := validation.ExpandTemplate(action.Exec, req.Params)
	if err != nil {
		s.sendError(enc, req.Secret, "TEMPLATE_ERROR", err.Error())
		s.logger.Printf("[%s] rpc: %s failed (template error)", time.Now().Format("15:04:05"), req.Action)
		return
	}

	// Sync-wait: ensure inbound changes have landed before executing
	if s.syncWaiter != nil && s.sessionName != "" {
		syncLog := &logWriter{logger: s.logger}
		if err := fusesync.WaitForSyncWithLog(s.syncWaiter, s.sessionName, s.syncTimeout, syncLog); err != nil {
			s.logger.Printf("[%s] rpc: %s sync-wait error: %v", time.Now().Format("15:04:05"), req.Action, err)
		}
	}

	// Determine timeout
	timeout := 600
	if action.Timeout > 0 {
		timeout = action.Timeout
	}

	// Execute
	result := Execute(ExecConfig{
		Command:    expanded,
		WorkDir:    s.projectDir,
		Timeout:    time.Duration(timeout) * time.Second,
		Secret:     req.Secret,
		Encoder:    enc,
	})

	// Record last action
	s.mu.Lock()
	s.lastAction = &LastAction{
		Name:      req.Action,
		ExitCode:  result.ExitCode,
		Duration:  result.Duration.Milliseconds(),
		Timestamp: time.Now(),
	}
	s.mu.Unlock()

	// Send exit
	enc.Send(rpc.ExitMessage{
		Type:     rpc.TypeExit,
		Secret:   req.Secret,
		Code:     result.ExitCode,
		Duration: result.Duration.Milliseconds(),
	})

	status := "completed"
	if result.ExitCode != 0 {
		status = "failed"
	}
	s.logger.Printf("[%s] rpc: %s %s (exit=%d, %dms)",
		time.Now().Format("15:04:05"), req.Action, status, result.ExitCode, result.Duration.Milliseconds())
}

func (s *Server) handleActions(req rpc.ActionsRequest, enc *rpc.Encoder) {
	var actions []rpc.ActionInfo

	// Sort action names for deterministic output
	names := make([]string, 0, len(s.config.Actions))
	for name := range s.config.Actions {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		action := s.config.Actions[name]
		info := rpc.ActionInfo{
			Name:        name,
			Description: action.Description,
		}

		if len(action.Params) > 0 {
			info.Params = make(map[string]rpc.ParamSchema)
			for pName, p := range action.Params {
				schema := rpc.ParamSchema{Type: p.Type}
				if p.Pattern != "" {
					schema.Pattern = p.Pattern
				}
				if len(p.Values) > 0 {
					schema.Values = p.Values
				}
				if len(p.Range) == 2 {
					min, max := p.Range[0], p.Range[1]
					schema.Min = &min
					schema.Max = &max
				}
				info.Params[pName] = schema
			}
		}

		actions = append(actions, info)
	}

	enc.Send(rpc.ActionsResponse{
		Type:    rpc.TypeActionsResponse,
		Secret:  req.Secret,
		Actions: actions,
	})
}

// logWriter adapts a *log.Logger to io.Writer for sync-wait log output.
type logWriter struct {
	logger *log.Logger
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.logger.Print(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func (s *Server) sendError(enc *rpc.Encoder, secret, code, message string) {
	enc.Send(rpc.ErrorResponse{
		Type:    rpc.TypeError,
		Secret:  secret,
		Code:    code,
		Message: message,
	})
}
