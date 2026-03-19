package server

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type hookInput struct {
	ToolName  string    `json:"tool_name"`
	ToolInput toolInput `json:"tool_input"`
}

type toolInput struct {
	Command  string `json:"command"`
	FilePath string `json:"file_path"`
	Pattern  string `json:"pattern"`
}

// extractDetail returns a brief description of what a tool did.
func extractDetail(input hookInput) string {
	switch input.ToolName {
	case "Bash":
		cmd := input.ToolInput.Command
		if cmd == "" {
			return ""
		}
		if idx := strings.IndexByte(cmd, '\n'); idx >= 0 {
			cmd = cmd[:idx]
		}
		if len(cmd) > 30 {
			cmd = cmd[:30]
		}
		return cmd
	case "Edit", "Read", "Write":
		fp := input.ToolInput.FilePath
		if fp == "" {
			return ""
		}
		return filepath.Base(fp)
	case "Grep", "Glob":
		p := input.ToolInput.Pattern
		if len(p) > 20 {
			p = p[:20]
		}
		return p
	case "Agent":
		return "subagent"
	default:
		return ""
	}
}

// CmdHook handles PostToolUse hook events from Claude Code.
// Reads JSON from stdin, writes activity to /tmp/work-cli/<session>.json.
func CmdHook() {
	statusDir := "/tmp/work-cli"
	os.MkdirAll(statusDir, 0755)

	// Detect session name: prefer env var, fall back to tmux pane lookup
	session := os.Getenv("WORK_SESSION")
	if session == "" {
		if pane := os.Getenv("TMUX_PANE"); pane != "" {
			out, err := tmuxRun("display-message", "-p", "-t", pane, "#{session_name}")
			if err == nil && out != "" {
				session = strings.TrimSpace(out)
			}
		}
	}
	if session == "" {
		return
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}

	var input hookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return
	}

	detail := extractDetail(input)

	activity := map[string]any{
		"tool":   input.ToolName,
		"detail": detail,
		"ts":     time.Now().Unix(),
	}

	activityData, _ := json.Marshal(activity)
	os.WriteFile(filepath.Join(statusDir, session+".json"), activityData, 0644)
}
