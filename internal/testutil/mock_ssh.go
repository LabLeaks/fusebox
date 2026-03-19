package testutil

import (
	"fmt"
	"os/exec"
	"sync"
	"time"
)


// MockSSH implements ssh.Runner for testing.
// Register responses with On() and verify calls with Calls().
type MockSSH struct {
	mu        sync.Mutex
	responses map[string]mockResponse
	calls     []string
}

type mockResponse struct {
	output []byte
	err    error
	delay  time.Duration
}

func NewMockSSH() *MockSSH {
	return &MockSSH{
		responses: make(map[string]mockResponse),
	}
}

// On registers a response for a given command.
func (m *MockSSH) On(command string, output []byte, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[command] = mockResponse{output: output, err: err}
}

// OnJSON is a convenience for registering JSON responses.
func (m *MockSSH) OnJSON(command, json string) {
	m.On(command, []byte(json), nil)
}

// OnDelayed registers a response with a delay.
func (m *MockSSH) OnDelayed(command string, output []byte, err error, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[command] = mockResponse{output: output, err: err, delay: delay}
}

// OnJSONDelayed registers a JSON response with a delay.
func (m *MockSSH) OnJSONDelayed(command, json string, delay time.Duration) {
	m.OnDelayed(command, []byte(json), nil, delay)
}

// OnError registers an error response for a given command.
func (m *MockSSH) OnError(command string, err error) {
	m.On(command, nil, err)
}

// Run returns the registered response for the command.
func (m *MockSSH) Run(command string) ([]byte, error) {
	m.mu.Lock()
	m.calls = append(m.calls, command)

	resp, ok := m.responses[command]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("unexpected command: %q", command)
	}
	delay := resp.delay
	m.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}
	return resp.output, resp.err
}

// AttachCmd returns a no-op command (echo) for testing.
func (m *MockSSH) AttachCmd(session string) *exec.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "attach:"+session)
	return exec.Command("echo", "attached to "+session)
}

// AttachPaneCmd returns a no-op command for testing pane attachment.
func (m *MockSSH) AttachPaneCmd(session string, pane int) *exec.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, fmt.Sprintf("attach-pane:%s:%d", session, pane))
	return exec.Command("echo", fmt.Sprintf("attached to %s pane %d", session, pane))
}

// Calls returns all commands that were executed.
func (m *MockSSH) Calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.calls))
	copy(result, m.calls)
	return result
}

// Called returns true if the given command was executed.
func (m *MockSSH) Called(command string) bool {
	for _, c := range m.Calls() {
		if c == command {
			return true
		}
	}
	return false
}

// HasResponse returns true if a response is registered for the given command.
func (m *MockSSH) HasResponse(command string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.responses[command]
	return ok
}

// Reset clears all registered responses and call history.
func (m *MockSSH) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = make(map[string]mockResponse)
	m.calls = nil
}
