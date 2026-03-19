//go:build !linux

package sandbox

import "fmt"

var errNotLinux = fmt.Errorf("sandbox isolation requires Linux")

// CanSandbox returns false on non-Linux platforms.
func CanSandbox() (bool, string) {
	return false, "sandbox isolation requires Linux"
}

// Up is not supported on non-Linux platforms.
func (s *Sandbox) Up(cfg Config) error { return errNotLinux }

// Down is not supported on non-Linux platforms.
func (s *Sandbox) Down() error { return errNotLinux }

// Status returns not-running on non-Linux platforms.
func (s *Sandbox) Status() (Status, error) { return Status{}, nil }

// Exec is not supported on non-Linux platforms.
func (s *Sandbox) Exec(args []string) ([]byte, error) { return nil, errNotLinux }
