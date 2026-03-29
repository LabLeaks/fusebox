package rpc

// MessageType identifies the kind of RPC message.
type MessageType string

const (
	TypeExec            MessageType = "exec"
	TypeStdout          MessageType = "stdout"
	TypeStderr          MessageType = "stderr"
	TypeExit            MessageType = "exit"
	TypeActions         MessageType = "actions"
	TypeActionsResponse MessageType = "actions_response"
	TypeError           MessageType = "error"
)

// ExecRequest asks the local daemon to run a named action.
type ExecRequest struct {
	Type   MessageType       `json:"type"`
	Secret string            `json:"secret"`
	Action string            `json:"action"`
	Params map[string]string `json:"params,omitempty"`
}

// StdoutMessage streams a line of stdout from the action.
type StdoutMessage struct {
	Type   MessageType `json:"type"`
	Secret string      `json:"secret"`
	Line   string      `json:"line"`
}

// StderrMessage streams a line of stderr from the action.
type StderrMessage struct {
	Type   MessageType `json:"type"`
	Secret string      `json:"secret"`
	Line   string      `json:"line"`
}

// ExitMessage signals the action completed.
type ExitMessage struct {
	Type     MessageType `json:"type"`
	Secret   string      `json:"secret"`
	Code     int         `json:"code"`
	Duration int64       `json:"duration_ms"`
}

// ActionsRequest asks the daemon to list available actions.
type ActionsRequest struct {
	Type   MessageType `json:"type"`
	Secret string      `json:"secret"`
}

// ActionInfo describes a single action in an ActionsResponse.
type ActionInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Params      map[string]ParamSchema `json:"params,omitempty"`
}

// ParamSchema describes a parameter's validation rules.
type ParamSchema struct {
	Type    string   `json:"type"`
	Pattern string   `json:"pattern,omitempty"`
	Values  []string `json:"values,omitempty"`
	Min     *int     `json:"min,omitempty"`
	Max     *int     `json:"max,omitempty"`
}

// ActionsResponse lists available actions.
type ActionsResponse struct {
	Type    MessageType  `json:"type"`
	Secret  string       `json:"secret"`
	Actions []ActionInfo `json:"actions"`
}

// ErrorResponse signals an error.
type ErrorResponse struct {
	Type    MessageType `json:"type"`
	Secret  string      `json:"secret"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
}
