package server

import "fmt"

// doPreview captures tmux pane content. Returns the output or an error.
func doPreview(name, lines string) (string, error) {
	if !tmuxHasSession(name) {
		return "", fmt.Errorf("session not found: %s", name)
	}
	out, err := tmuxRun("capture-pane", "-t", name, "-p", "-S", fmt.Sprintf("-%s", lines))
	if err != nil {
		return "", fmt.Errorf("capture pane: %s", err.Error())
	}
	return out, nil
}

// CmdPreview captures and outputs tmux pane content.
func CmdPreview(name, lines string) {
	out, err := doPreview(name, lines)
	if err != nil {
		ExitError(err.Error())
	}
	fmt.Println(out)
}
