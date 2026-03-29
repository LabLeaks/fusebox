package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// StatusInfo is the JSON response served on the status socket.
type StatusInfo struct {
	Project    string      `json:"project"`
	Server     string      `json:"server,omitempty"`
	Container  string      `json:"container,omitempty"`
	SyncState  string      `json:"sync_state,omitempty"`
	LastAction *LastAction `json:"last_action,omitempty"`
}

// StatusServer serves project status over a Unix socket.
type StatusServer struct {
	listener net.Listener
	sockPath string
	getInfo  func() StatusInfo
	done     chan struct{}
}

// StatusSocketPath returns the path for a project's status socket.
func StatusSocketPath(project string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".fusebox", "run", project+".sock"), nil
}

// NewStatusServer creates a Unix socket status server at the given path.
func NewStatusServer(sockPath string, getInfo func() StatusInfo) (*StatusServer, error) {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(sockPath), 0700); err != nil {
		return nil, fmt.Errorf("creating socket directory: %w", err)
	}

	// Remove stale socket
	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", sockPath, err)
	}

	return &StatusServer{
		listener: listener,
		sockPath: sockPath,
		getInfo:  getInfo,
		done:     make(chan struct{}),
	}, nil
}

// Serve accepts connections and returns status JSON until closed.
func (s *StatusServer) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}

		info := s.getInfo()
		data, _ := json.Marshal(info)
		conn.Write(data)
		conn.Write([]byte("\n"))
		conn.Close()
	}
}

// Close tears down the status socket.
func (s *StatusServer) Close() error {
	close(s.done)
	err := s.listener.Close()
	os.Remove(s.sockPath)
	return err
}
