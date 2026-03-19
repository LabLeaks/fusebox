//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Up starts the sandbox: ensures rootfs, mounts overlay, launches the
// container namespace with tmux inside.
func (s *Sandbox) Up(cfg Config) error {
	if st, err := s.Status(); err == nil && st.Running {
		return nil // already running
	}

	if err := os.MkdirAll(s.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Ensure rootfs is downloaded
	if err := s.EnsureRootfs(); err != nil {
		return err
	}

	// Mount OverlayFS
	if err := s.mountOverlay(); err != nil {
		return fmt.Errorf("mount overlay: %w", err)
	}

	// Set up bind mounts inside the merged rootfs
	for _, bm := range cfg.BindMounts {
		target := filepath.Join(s.MergedDir(), bm.Container)
		if err := os.MkdirAll(target, 0755); err != nil {
			return fmt.Errorf("create bind target %s: %w", bm.Container, err)
		}
		flags := uintptr(syscall.MS_BIND | syscall.MS_REC)
		if bm.ReadOnly {
			flags |= syscall.MS_RDONLY
		}
		if err := syscall.Mount(bm.Host, target, "", flags, ""); err != nil {
			return fmt.Errorf("bind mount %s -> %s: %w", bm.Host, bm.Container, err)
		}
	}

	// Bind-mount the tmux socket directory
	sockDir := filepath.Dir(s.TmuxSocket())
	if err := os.MkdirAll(sockDir, 0755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}

	// Launch the init process in a new namespace via re-exec
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	cmd := exec.Command(exe, "init")
	cmd.Env = append(os.Environ(),
		"FUSEBOX_DATA_DIR="+s.DataDir,
		"FUSEBOX_ROOTFS="+s.MergedDir(),
		"FUSEBOX_SOCKET="+s.TmuxSocket(),
		"FUSEBOX_HOSTNAME="+cfg.Hostname,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		s.unmountOverlay()
		return fmt.Errorf("start sandbox init: %w", err)
	}

	// Write PID file
	pidFile := filepath.Join(s.DataDir, "sandbox.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

	// Don't wait — init runs as a daemon
	go cmd.Wait()

	return nil
}

// Down stops the sandbox, killing the init process and unmounting.
func (s *Sandbox) Down() error {
	st, err := s.Status()
	if err != nil {
		return err
	}
	if !st.Running {
		return nil
	}

	// Kill the init process
	proc, err := os.FindProcess(st.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", st.PID, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Try SIGKILL
		proc.Kill()
	}
	proc.Wait()

	// Clean up PID file
	os.Remove(filepath.Join(s.DataDir, "sandbox.pid"))

	// Unmount overlay
	s.unmountOverlay()

	return nil
}

// Status checks whether the sandbox is running.
func (s *Sandbox) Status() (Status, error) {
	pidFile := filepath.Join(s.DataDir, "sandbox.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return Status{}, nil
		}
		return Status{}, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return Status{}, nil
	}

	// Check if process is still alive
	proc, err := os.FindProcess(pid)
	if err != nil {
		return Status{}, nil
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process not running — clean up stale PID file
		os.Remove(pidFile)
		return Status{}, nil
	}

	return Status{
		Running: true,
		PID:     pid,
		Socket:  s.TmuxSocket(),
	}, nil
}

// Exec runs a command inside the sandbox namespace and returns its output.
func (s *Sandbox) Exec(args []string) ([]byte, error) {
	st, err := s.Status()
	if err != nil {
		return nil, err
	}
	if !st.Running {
		return nil, fmt.Errorf("sandbox not running")
	}

	// Use nsenter to run command in the sandbox's namespaces
	nsenterArgs := []string{
		"-t", strconv.Itoa(st.PID),
		"-m", "-p", "-U",
		"--",
	}
	nsenterArgs = append(nsenterArgs, args...)

	cmd := exec.Command("nsenter", nsenterArgs...)
	return cmd.CombinedOutput()
}
