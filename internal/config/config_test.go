package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Host != "" {
		t.Errorf("expected empty default host, got %s", cfg.Server.Host)
	}
	if cfg.Server.User != "" {
		t.Errorf("expected empty default user, got %s", cfg.Server.User)
	}
	if cfg.Claude.Flags == "" {
		t.Error("expected non-empty default claude flags")
	}
}

func TestValidate_MissingHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.User = "someone"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing host")
	}
}

func TestValidate_MissingUser(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Host = "myhost"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing user")
	}
}

func TestValidate_OK(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Host = "myhost"
	cfg.Server.User = "myuser"
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveHomeDir(t *testing.T) {
	cfg := Config{Server: Server{User: "alice"}}
	if got := cfg.ResolveHomeDir(); got != "/home/alice" {
		t.Errorf("expected /home/alice, got %s", got)
	}

	cfg.Server.HomeDir = "/custom/home"
	if got := cfg.ResolveHomeDir(); got != "/custom/home" {
		t.Errorf("expected /custom/home, got %s", got)
	}
}

func TestResolveServerPath(t *testing.T) {
	cfg := Config{Server: Server{User: "alice"}}
	if got := cfg.ResolveServerPath(); got != "/home/alice/bin/work" {
		t.Errorf("expected /home/alice/bin/work, got %s", got)
	}

	cfg.ServerPath = "/opt/bin/work"
	if got := cfg.ResolveServerPath(); got != "/opt/bin/work" {
		t.Errorf("expected /opt/bin/work, got %s", got)
	}
}

func TestLoadFrom_FileNotFound(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	// Should return defaults (empty host/user)
	if cfg.Server.Host != "" {
		t.Errorf("expected empty default host, got %s", cfg.Server.Host)
	}
}

func TestLoadFrom_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `server:
  host: custom-host
  user: custom-user
claude:
  flags: "--custom-flag"
browse_roots:
  - ~/work/test
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Host != "custom-host" {
		t.Errorf("expected custom-host, got %s", cfg.Server.Host)
	}
	if cfg.Server.User != "custom-user" {
		t.Errorf("expected custom-user, got %s", cfg.Server.User)
	}
	if cfg.Claude.Flags != "--custom-flag" {
		t.Errorf("expected --custom-flag, got %s", cfg.Claude.Flags)
	}
	if len(cfg.BrowseRoots) != 1 || cfg.BrowseRoots[0] != "~/work/test" {
		t.Errorf("unexpected browse roots: %v", cfg.BrowseRoots)
	}
}

func TestLoadFrom_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFrom(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestSaveTo_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yaml")

	original := Config{
		Server: Server{Host: "myhost", User: "myuser", HomeDir: "/custom"},
		Claude: Claude{Flags: "--test-flag"},
		BrowseRoots: []string{"~/work", "~/src"},
	}

	if err := SaveTo(original, path); err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	if loaded.Server.Host != original.Server.Host {
		t.Errorf("host: got %s, want %s", loaded.Server.Host, original.Server.Host)
	}
	if loaded.Server.User != original.Server.User {
		t.Errorf("user: got %s, want %s", loaded.Server.User, original.Server.User)
	}
	if loaded.Server.HomeDir != original.Server.HomeDir {
		t.Errorf("home_dir: got %s, want %s", loaded.Server.HomeDir, original.Server.HomeDir)
	}
	if loaded.Claude.Flags != original.Claude.Flags {
		t.Errorf("flags: got %s, want %s", loaded.Claude.Flags, original.Claude.Flags)
	}
	if len(loaded.BrowseRoots) != 2 || loaded.BrowseRoots[0] != "~/work" || loaded.BrowseRoots[1] != "~/src" {
		t.Errorf("browse_roots: got %v, want %v", loaded.BrowseRoots, original.BrowseRoots)
	}
}

func TestLoadFrom_PartialOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `server:
  host: my-server
  user: myuser
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Host != "my-server" {
		t.Errorf("expected my-server, got %s", cfg.Server.Host)
	}
	if cfg.Server.User != "myuser" {
		t.Errorf("expected myuser, got %s", cfg.Server.User)
	}
	// Claude flags should retain default
	if cfg.Claude.Flags == "" {
		t.Error("expected default claude flags to be retained")
	}
}
