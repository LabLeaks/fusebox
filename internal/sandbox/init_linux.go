//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// RunInit is the entry point for the sandbox init process (PID 1 inside namespace).
// Called when the binary is re-exec'd with "init" as the first argument.
func RunInit() {
	rootfs := os.Getenv("FUSEBOX_ROOTFS")
	socket := os.Getenv("FUSEBOX_SOCKET")
	hostname := os.Getenv("FUSEBOX_HOSTNAME")

	if rootfs == "" || socket == "" {
		fmt.Fprintln(os.Stderr, "fusebox init: missing FUSEBOX_ROOTFS or FUSEBOX_SOCKET")
		os.Exit(1)
	}

	if err := initMain(rootfs, socket, hostname); err != nil {
		fmt.Fprintf(os.Stderr, "fusebox init: %v\n", err)
		os.Exit(1)
	}
}

func initMain(rootfs, socket, hostname string) error {
	// Set hostname
	if hostname != "" {
		syscall.Sethostname([]byte(hostname))
	}

	// Pivot root to the OverlayFS merged dir
	if err := pivotRoot(rootfs); err != nil {
		return fmt.Errorf("pivot root: %w", err)
	}

	// Mount /proc inside the namespace
	os.MkdirAll("/proc", 0755)
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("mount proc: %w", err)
	}

	// Mount /dev/pts for tmux
	os.MkdirAll("/dev/pts", 0755)
	syscall.Mount("devpts", "/dev/pts", "devpts", 0, "newinstance,ptmxmode=0666")

	// Mount /tmp
	os.MkdirAll("/tmp", 0777)
	syscall.Mount("tmpfs", "/tmp", "tmpfs", 0, "")

	// Start tmux server with the designated socket
	// The socket is bind-mounted from the host so the TUI can find it
	tmuxCmd := exec.Command("tmux", "-S", socket, "start-server")
	tmuxCmd.Stdout = os.Stdout
	tmuxCmd.Stderr = os.Stderr
	if err := tmuxCmd.Start(); err != nil {
		return fmt.Errorf("start tmux: %w", err)
	}
	tmuxPID := tmuxCmd.Process.Pid

	// PID 1 zombie reaping loop
	var status syscall.WaitStatus
	for {
		pid, err := syscall.Wait4(-1, &status, 0, nil)
		if pid == tmuxPID || err != nil {
			break
		}
	}

	return nil
}

func pivotRoot(newroot string) error {
	// Bind mount newroot onto itself (required for pivot_root)
	if err := syscall.Mount(newroot, newroot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount new root: %w", err)
	}

	putold := newroot + "/.pivot_old"
	os.MkdirAll(putold, 0700)

	if err := syscall.PivotRoot(newroot, putold); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}

	if err := os.Chdir("/"); err != nil {
		return err
	}

	// Unmount old root
	if err := syscall.Unmount("/.pivot_old", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old root: %w", err)
	}
	os.RemoveAll("/.pivot_old")

	return nil
}
