package sync

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const mutagenVersion = "0.18.1"

// installMutagen downloads the mutagen binary to the given directory.
func installMutagen(binDir string) error {
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	arch := runtime.GOARCH
	goos := runtime.GOOS

	url := fmt.Sprintf(
		"https://github.com/mutagen-io/mutagen/releases/download/v%s/mutagen_%s_%s_v%s.tar.gz",
		mutagenVersion, goos, arch, mutagenVersion,
	)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download mutagen: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download mutagen: HTTP %d", resp.StatusCode)
	}

	// Download to temp file
	tmpFile, err := os.CreateTemp(binDir, "mutagen-download-*.tar.gz")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download mutagen: %w", err)
	}
	tmpFile.Close()

	// Extract the mutagen binary from the tarball
	destPath := filepath.Join(binDir, "mutagen")
	if err := extractMutagenFromTarball(tmpPath, destPath); err != nil {
		return fmt.Errorf("extract mutagen: %w", err)
	}

	return nil
}

// extractMutagenFromTarball extracts the mutagen binary using tar.
func extractMutagenFromTarball(tarball, dest string) error {
	dir := filepath.Dir(dest)

	// Use system tar to extract — simpler than pulling in archive/tar + compress/gzip
	cmd := newExecCommand("tar", "xzf", tarball, "-C", dir, "mutagen")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Try without specifying the file (some tarballs have different structure)
		cmd2 := newExecCommand("tar", "xzf", tarball, "-C", dir)
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("tar extract: %s / %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	_ = out

	if err := os.Chmod(dest, 0755); err != nil {
		return fmt.Errorf("chmod mutagen: %w", err)
	}

	return nil
}
