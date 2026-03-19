package sandbox

import "path/filepath"

// Sandbox manages an isolated namespace on the server.
type Sandbox struct {
	DataDir string // ~/.fusebox
}

// Config holds sandbox creation parameters.
type Config struct {
	BindMounts []BindMount
	Hostname   string
}

// BindMount maps a host path into the container.
type BindMount struct {
	Host      string
	Container string
	ReadOnly  bool
}

// Status describes the current state of the sandbox.
type Status struct {
	Running bool
	PID     int
	Socket  string
}

// New creates a Sandbox manager rooted at dataDir.
func New(dataDir string) *Sandbox {
	return &Sandbox{DataDir: dataDir}
}

// TmuxSocket returns the path to the sandbox tmux socket on the host.
func (s *Sandbox) TmuxSocket() string {
	return filepath.Join(s.DataDir, "tmux.sock")
}

// RootfsDir returns the path where the rootfs tarball is extracted.
func (s *Sandbox) RootfsDir() string {
	return filepath.Join(s.DataDir, "rootfs")
}

// OverlayDir returns the base path for OverlayFS layers.
func (s *Sandbox) OverlayDir() string {
	return filepath.Join(s.DataDir, "overlay")
}

// MergedDir returns the OverlayFS merged view path.
func (s *Sandbox) MergedDir() string {
	return filepath.Join(s.OverlayDir(), "merged")
}
