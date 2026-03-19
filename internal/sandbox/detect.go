//go:build linux

package sandbox

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// CanSandbox checks whether the system supports namespace isolation.
// Returns true if supported, or false with a reason string.
func CanSandbox() (bool, string) {
	// Check kernel version ≥ 5.11 (needed for rootless OverlayFS)
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return false, "cannot read kernel version"
	}

	// Convert [65]int8 to string
	var relBuf []byte
	for _, b := range uname.Release {
		if b == 0 {
			break
		}
		relBuf = append(relBuf, byte(b))
	}
	release := string(relBuf)
	major, minor := parseKernelVersion(release)
	if major < 5 || (major == 5 && minor < 11) {
		return false, "kernel " + release + " too old (need ≥5.11 for rootless OverlayFS)"
	}

	// Check user namespaces enabled
	data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone")
	if err == nil {
		val := strings.TrimSpace(string(data))
		if val == "0" {
			return false, "user namespaces disabled (unprivileged_userns_clone=0)"
		}
	}
	// If file doesn't exist, user namespaces are likely enabled by default

	// Check max_user_namespaces > 0
	data, err = os.ReadFile("/proc/sys/user/max_user_namespaces")
	if err == nil {
		val := strings.TrimSpace(string(data))
		if val == "0" {
			return false, "user namespaces disabled (max_user_namespaces=0)"
		}
	}

	return true, ""
}

func parseKernelVersion(release string) (major, minor int) {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		// Minor might have suffix like "11-generic"
		minorStr := strings.SplitN(parts[1], "-", 2)[0]
		minor, _ = strconv.Atoi(minorStr)
	}
	return
}
