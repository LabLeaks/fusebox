package config

import (
	"os"
	"path/filepath"
	"testing"
)

// helper: create a temp dir tree with fusebox.yaml and global config.
func setupResolverFixtures(t *testing.T) (projectDir, subDir, globalPath string) {
	t.Helper()

	projectDir = t.TempDir()

	// Write a minimal fusebox.yaml in the project root.
	fuseboxYAML := `version: 1
actions:
  build:
    exec: "make build"
`
	if err := os.WriteFile(filepath.Join(projectDir, "fusebox.yaml"), []byte(fuseboxYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a nested subdirectory (no fusebox.yaml here).
	subDir = filepath.Join(projectDir, "src", "pkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a global config to a temp path.
	globalDir := t.TempDir()
	globalPath = filepath.Join(globalDir, "config")
	globalYAML := `server:
  host: "test-server"
  user: "deploy"
  port: 2222
token: "test-token"
defaults:
  rpc_port: 9999
`
	if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
		t.Fatal(err)
	}

	return projectDir, subDir, globalPath
}

func TestResolveFromProjectRoot(t *testing.T) {
	projectDir, _, globalPath := setupResolverFixtures(t)

	cfg, err := Resolve(ResolveOptions{
		StartDir:   projectDir,
		GlobalPath: globalPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ProjectRoot != projectDir {
		t.Errorf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectDir)
	}
	if cfg.Project.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Project.Version)
	}
	if cfg.Server.Host != "test-server" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "test-server")
	}
	if cfg.Server.User != "deploy" {
		t.Errorf("Server.User = %q, want %q", cfg.Server.User, "deploy")
	}
	if cfg.Server.Port != 2222 {
		t.Errorf("Server.Port = %d, want 2222", cfg.Server.Port)
	}
	if cfg.Token != "test-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "test-token")
	}
	if cfg.Defaults.RPCPort != 9999 {
		t.Errorf("Defaults.RPCPort = %d, want 9999", cfg.Defaults.RPCPort)
	}
}

func TestResolveWalksUpFromSubdir(t *testing.T) {
	projectDir, subDir, globalPath := setupResolverFixtures(t)

	cfg, err := Resolve(ResolveOptions{
		StartDir:   subDir,
		GlobalPath: globalPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ProjectRoot != projectDir {
		t.Errorf("ProjectRoot = %q, want %q", cfg.ProjectRoot, projectDir)
	}
}

func TestResolveServerOverride(t *testing.T) {
	projectDir, _, globalPath := setupResolverFixtures(t)

	cfg, err := Resolve(ResolveOptions{
		StartDir:       projectDir,
		GlobalPath:     globalPath,
		ServerOverride: "override-host",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Host != "override-host" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "override-host")
	}
	// User and port still come from global config.
	if cfg.Server.User != "deploy" {
		t.Errorf("Server.User = %q, want %q", cfg.Server.User, "deploy")
	}
}

func TestResolveNoFuseboxYAML(t *testing.T) {
	emptyDir := t.TempDir()

	_, err := Resolve(ResolveOptions{
		StartDir:   emptyDir,
		GlobalPath: "/nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for missing fusebox.yaml")
	}

	want := "not in a fusebox project (no fusebox.yaml found)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveNoGlobalConfig(t *testing.T) {
	projectDir, _, _ := setupResolverFixtures(t)

	_, err := Resolve(ResolveOptions{
		StartDir:   projectDir,
		GlobalPath: "/nonexistent/config",
	})
	if err == nil {
		t.Fatal("expected error for missing global config")
	}
}

func TestFindProjectRootAtRoot(t *testing.T) {
	// Searching from filesystem root should fail cleanly.
	_, err := findProjectRoot("/")
	if err == nil {
		t.Fatal("expected error when no fusebox.yaml exists at /")
	}
}
