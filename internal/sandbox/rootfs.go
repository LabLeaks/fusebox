package sandbox

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	rootfsRepo    = "lableaks/fusebox"
	rootfsVersion = "latest"
)

// EnsureRootfs downloads and extracts the rootfs tarball if not present.
func (s *Sandbox) EnsureRootfs() error {
	marker := filepath.Join(s.RootfsDir(), ".ready")
	if _, err := os.Stat(marker); err == nil {
		return nil // already extracted
	}

	if err := os.MkdirAll(s.RootfsDir(), 0755); err != nil {
		return fmt.Errorf("create rootfs dir: %w", err)
	}

	arch := runtime.GOARCH
	url := fmt.Sprintf(
		"https://github.com/%s/releases/%s/download/fusebox-rootfs-%s.tar.gz",
		rootfsRepo, rootfsVersion, arch,
	)

	fmt.Printf("Downloading rootfs (%s)...\n", arch)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download rootfs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download rootfs: HTTP %d from %s", resp.StatusCode, url)
	}

	// Stream to temp file
	tmpFile, err := os.CreateTemp(s.DataDir, "rootfs-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download rootfs: %w", err)
	}
	tmpFile.Close()

	fmt.Println("Extracting rootfs...")

	// Extract tarball
	cmd := exec.Command("tar", "xzf", tmpPath, "-C", s.RootfsDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract rootfs: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Write marker
	if err := os.WriteFile(marker, []byte(arch+"\n"), 0644); err != nil {
		return fmt.Errorf("write rootfs marker: %w", err)
	}

	fmt.Println("Rootfs ready.")
	return nil
}
