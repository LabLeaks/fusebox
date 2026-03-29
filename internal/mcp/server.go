package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// DaemonClient abstracts the RPC operations the MCP server needs.
type DaemonClient interface {
	RequestActions() ([]ActionDescriptor, error)
	Exec(action string, params map[string]string) (*ExecResult, error)
	Close() error
}

// ActionDescriptor describes an action available from the daemon.
type ActionDescriptor struct {
	Name        string
	Description string
	Params      map[string]ParamDescriptor
}

// ParamDescriptor describes a parameter's validation rules.
type ParamDescriptor struct {
	Type    string
	Pattern string
	Values  []string
	Min     *int
	Max     *int
}

// ExecResult holds the output of a completed action execution.
type ExecResult struct {
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// --- JSON-RPC 2.0 types ---

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Server is an MCP server that reads JSON-RPC from stdin and writes to stdout.
type Server struct {
	client DaemonClient
	in     io.Reader
	out    io.Writer
}

// NewServer creates an MCP server backed by the given daemon client.
func NewServer(client DaemonClient, in io.Reader, out io.Writer) *Server {
	return &Server{client: client, in: in, out: out}
}

// Serve reads JSON-RPC requests line by line and dispatches them.
// It returns when stdin is closed (EOF).
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.in)
	// MCP messages can be large (tool results with output).
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "parse error")
			continue
		}

		s.dispatch(&req)
	}

	return scanner.Err()
}

func (s *Server) dispatch(req *jsonrpcRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// Client notification, no response needed.
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req *jsonrpcRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "fusebox",
			"version": "0.1.0",
		},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *jsonrpcRequest) {
	actions, err := s.client.RequestActions()
	if err != nil {
		if strings.Contains(err.Error(), "unreachable") || strings.Contains(err.Error(), "connection refused") {
			s.sendResult(req.ID, map[string]interface{}{"tools": []interface{}{}})
			return
		}
		s.sendError(req.ID, -32603, fmt.Sprintf("failed to list actions: %v", err))
		return
	}

	tools := make([]interface{}, 0, len(actions))
	for _, a := range actions {
		tools = append(tools, ActionToTool(a))
	}

	s.sendResult(req.ID, map[string]interface{}{"tools": tools})
}

func (s *Server) handleToolsCall(req *jsonrpcRequest) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "invalid params")
			return
		}
	}

	// Strip fusebox_ prefix to get action name.
	actionName := strings.TrimPrefix(params.Name, "fusebox_")

	// Convert arguments to string map.
	strParams := make(map[string]string, len(params.Arguments))
	for k, v := range params.Arguments {
		strParams[k] = fmt.Sprintf("%v", v)
	}

	result, err := s.client.Exec(actionName, strParams)
	if err != nil {
		if strings.Contains(err.Error(), "unreachable") || strings.Contains(err.Error(), "connection refused") {
			s.sendToolResult(req.ID, true, "local machine unreachable")
			return
		}
		s.sendToolResult(req.ID, true, fmt.Sprintf("execution error: %v", err))
		return
	}

	content, _ := json.Marshal(result)
	isError := result.ExitCode != 0
	s.sendToolResult(req.ID, isError, string(content))
}

func (s *Server) sendToolResult(id json.RawMessage, isError bool, text string) {
	result := map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
		"isError": isError,
	}
	s.sendResult(id, result)
}

func (s *Server) sendResult(id json.RawMessage, result interface{}) {
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeResponse(resp)
}

func (s *Server) sendError(id json.RawMessage, code int, message string) {
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonrpcError{Code: code, Message: message},
	}
	s.writeResponse(resp)
}

func (s *Server) writeResponse(resp jsonrpcResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	data = append(data, '\n')
	s.out.Write(data) //nolint:errcheck
}
