package integration

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/daemon"
	"github.com/lableaks/fusebox/internal/rpc"
)

// fixturesDir returns the absolute path to test/fixtures/go-project.
func fixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "fixtures", "go-project")
}

// loadFixtureConfig loads the fusebox.yaml from the test fixture.
func loadFixtureConfig(t *testing.T) *config.ProjectConfig {
	t.Helper()
	cfg, err := config.LoadProjectConfig(filepath.Join(fixturesDir(), "fusebox.yaml"))
	if err != nil {
		t.Fatalf("loading fixture config: %v", err)
	}
	return cfg
}

// startDaemon starts a local daemon on a random port and returns the
// address, secret, and a cleanup function.
func startDaemon(t *testing.T, cfg *config.ProjectConfig) (addr, secret string, cleanup func()) {
	t.Helper()

	secret, err := rpc.GenerateSecret()
	if err != nil {
		t.Fatalf("generating secret: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	addr = listener.Addr().String()

	logger := log.New(os.Stderr, "[test-daemon] ", 0)

	srv := daemon.NewServer(listener, daemon.ServerConfig{
		Config:     cfg,
		Secret:     secret,
		ProjectDir: fixturesDir(),
		Logger:     logger,
	})

	go srv.Serve()

	cleanup = func() {
		srv.Close()
	}

	return addr, secret, cleanup
}

// requireRemoteEnv skips the test if FUSEBOX_TEST_HOST/FUSEBOX_TEST_USER are not set.
func requireRemoteEnv(t *testing.T) (host, user string) {
	t.Helper()
	host = os.Getenv("FUSEBOX_TEST_HOST")
	user = os.Getenv("FUSEBOX_TEST_USER")
	if host == "" || user == "" {
		t.Skip("FUSEBOX_TEST_HOST and FUSEBOX_TEST_USER not set, skipping remote integration test")
	}
	return host, user
}
