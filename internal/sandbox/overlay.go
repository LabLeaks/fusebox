//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// mountOverlay sets up an OverlayFS with:
//   - lower = rootfs (read-only base)
//   - upper = writable layer (container writes)
//   - work  = OverlayFS work directory
//   - merged = combined view
func (s *Sandbox) mountOverlay() error {
	dirs := map[string]string{
		"lower":  s.RootfsDir(),
		"upper":  filepath.Join(s.OverlayDir(), "upper"),
		"work":   filepath.Join(s.OverlayDir(), "work"),
		"merged": s.MergedDir(),
	}

	for name, path := range dirs {
		if name == "lower" {
			continue // already exists from rootfs extraction
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("create %s dir: %w", name, err)
		}
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		dirs["lower"], dirs["upper"], dirs["work"])

	if err := syscall.Mount("overlay", dirs["merged"], "overlay", 0, opts); err != nil {
		return fmt.Errorf("mount overlay: %w", err)
	}

	return nil
}

// unmountOverlay tears down the OverlayFS.
func (s *Sandbox) unmountOverlay() error {
	if err := syscall.Unmount(s.MergedDir(), 0); err != nil {
		return fmt.Errorf("unmount overlay: %w", err)
	}
	return nil
}
