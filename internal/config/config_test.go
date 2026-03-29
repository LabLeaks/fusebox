package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", name)
}

// --- ProjectConfig tests ---

func TestLoadValidProjectConfig(t *testing.T) {
	cfg, err := LoadProjectConfig(testdataPath("valid.fusebox.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}

	if len(cfg.Sync.Ignore) != 3 {
		t.Errorf("sync.ignore length = %d, want 3", len(cfg.Sync.Ignore))
	}

	if len(cfg.Actions) != 3 {
		t.Errorf("actions count = %d, want 3", len(cfg.Actions))
	}

	// Verify build action (no params)
	build, ok := cfg.Actions["build"]
	if !ok {
		t.Fatal("missing action: build")
	}
	if build.Exec != "make build" {
		t.Errorf("build.exec = %q, want %q", build.Exec, "make build")
	}
	if len(build.Params) != 0 {
		t.Errorf("build.params length = %d, want 0", len(build.Params))
	}

	// Verify test action (regex param)
	testAction := cfg.Actions["test"]
	if testAction.Timeout != 300 {
		t.Errorf("test.timeout = %d, want 300", testAction.Timeout)
	}
	pkg, ok := testAction.Params["package"]
	if !ok {
		t.Fatal("missing param: package")
	}
	if pkg.Type != "regex" {
		t.Errorf("package.type = %q, want %q", pkg.Type, "regex")
	}
	if pkg.Pattern != "^[a-zA-Z0-9_/]+$" {
		t.Errorf("package.pattern = %q", pkg.Pattern)
	}

	// Verify deploy action (enum + int params)
	deploy := cfg.Actions["deploy"]
	env := deploy.Params["env"]
	if env.Type != "enum" {
		t.Errorf("env.type = %q, want %q", env.Type, "enum")
	}
	if len(env.Values) != 2 {
		t.Errorf("env.values length = %d, want 2", len(env.Values))
	}

	replicas := deploy.Params["replicas"]
	if replicas.Type != "int" {
		t.Errorf("replicas.type = %q, want %q", replicas.Type, "int")
	}
	if replicas.Range[0] != 1 || replicas.Range[1] != 10 {
		t.Errorf("replicas.range = %v, want [1, 10]", replicas.Range)
	}
}

func TestParseProjectConfigMissingVersion(t *testing.T) {
	_, err := ParseProjectConfig([]byte("actions:\n  foo:\n    exec: echo\n"))
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestParseProjectConfigBadVersion(t *testing.T) {
	_, err := ParseProjectConfig([]byte("version: 99\n"))
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestParseProjectConfigMissingExec(t *testing.T) {
	data := []byte("version: 1\nactions:\n  bad:\n    description: no exec\n")
	_, err := ParseProjectConfig(data)
	if err == nil {
		t.Fatal("expected error for missing exec")
	}
}

func TestParseProjectConfigBadParamType(t *testing.T) {
	data := []byte("version: 1\nactions:\n  a:\n    exec: echo\n    params:\n      x:\n        type: unknown\n")
	_, err := ParseProjectConfig(data)
	if err == nil {
		t.Fatal("expected error for unknown param type")
	}
}

func TestParseProjectConfigRegexMissingPattern(t *testing.T) {
	data := []byte("version: 1\nactions:\n  a:\n    exec: echo\n    params:\n      x:\n        type: regex\n")
	_, err := ParseProjectConfig(data)
	if err == nil {
		t.Fatal("expected error for regex missing pattern")
	}
}

func TestParseProjectConfigEnumMissingValues(t *testing.T) {
	data := []byte("version: 1\nactions:\n  a:\n    exec: echo\n    params:\n      x:\n        type: enum\n")
	_, err := ParseProjectConfig(data)
	if err == nil {
		t.Fatal("expected error for enum missing values")
	}
}

func TestParseProjectConfigIntMissingRange(t *testing.T) {
	data := []byte("version: 1\nactions:\n  a:\n    exec: echo\n    params:\n      x:\n        type: int\n")
	_, err := ParseProjectConfig(data)
	if err == nil {
		t.Fatal("expected error for int missing range")
	}
}

func TestParseProjectConfigIntInvertedRange(t *testing.T) {
	data := []byte("version: 1\nactions:\n  a:\n    exec: echo\n    params:\n      x:\n        type: int\n        range: [10, 1]\n")
	_, err := ParseProjectConfig(data)
	if err == nil {
		t.Fatal("expected error for inverted range")
	}
}

func TestParseProjectConfigParamMissingType(t *testing.T) {
	data := []byte("version: 1\nactions:\n  a:\n    exec: echo\n    params:\n      x:\n        pattern: foo\n")
	_, err := ParseProjectConfig(data)
	if err == nil {
		t.Fatal("expected error for param missing type")
	}
}

func TestParseProjectConfigInvalidYAML(t *testing.T) {
	_, err := ParseProjectConfig([]byte(":\n  :\n  - {broken"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadProjectConfigMissingFile(t *testing.T) {
	_, err := LoadProjectConfig("/nonexistent/fusebox.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- GlobalConfig tests ---

func TestLoadValidGlobalConfig(t *testing.T) {
	cfg, err := LoadGlobalConfig(testdataPath("valid.global.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Host != "spotless-1" {
		t.Errorf("server.host = %q, want %q", cfg.Server.Host, "spotless-1")
	}
	if cfg.Server.User != "root" {
		t.Errorf("server.user = %q, want %q", cfg.Server.User, "root")
	}
	if cfg.Server.Port != 22 {
		t.Errorf("server.port = %d, want 22", cfg.Server.Port)
	}
	if cfg.Token != "sk-ant-test-token" {
		t.Errorf("token = %q", cfg.Token)
	}
	if cfg.Defaults.Image != "fusebox/claude:latest" {
		t.Errorf("defaults.image = %q", cfg.Defaults.Image)
	}
	if cfg.Defaults.RPCPort != 7600 {
		t.Errorf("defaults.rpc_port = %d, want 7600", cfg.Defaults.RPCPort)
	}
}

func TestParseGlobalConfigDefaults(t *testing.T) {
	data := []byte("server:\n  host: example.com\n  user: deploy\n")
	cfg, err := ParseGlobalConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 22 {
		t.Errorf("default port = %d, want 22", cfg.Server.Port)
	}
	if cfg.Defaults.Image != "fusebox/claude:latest" {
		t.Errorf("default image = %q", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "/bin/bash" {
		t.Errorf("default shell = %q", cfg.Defaults.Shell)
	}
	if cfg.Defaults.SyncTimeout != 30 {
		t.Errorf("default sync_timeout = %d", cfg.Defaults.SyncTimeout)
	}
	if cfg.Defaults.RPCPort != 7600 {
		t.Errorf("default rpc_port = %d", cfg.Defaults.RPCPort)
	}
}

func TestParseGlobalConfigMissingHost(t *testing.T) {
	data := []byte("server:\n  user: root\n")
	_, err := ParseGlobalConfig(data)
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestParseGlobalConfigMissingUser(t *testing.T) {
	data := []byte("server:\n  host: example.com\n")
	_, err := ParseGlobalConfig(data)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
}

func TestParseGlobalConfigInvalidYAML(t *testing.T) {
	_, err := ParseGlobalConfig([]byte(":\n  :\n  - {broken"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadGlobalConfigMissingFile(t *testing.T) {
	_, err := LoadGlobalConfig("/nonexistent/config")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateGlobalConfigDoesNotMutate(t *testing.T) {
	cfg := &GlobalConfig{
		Server: ServerConfig{Host: "example.com", User: "deploy"},
	}

	err := validateGlobalConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// validateGlobalConfig should NOT apply defaults
	if cfg.Server.Port != 0 {
		t.Errorf("port = %d after validate, want 0 (unmutated)", cfg.Server.Port)
	}
	if cfg.Defaults.Image != "" {
		t.Errorf("image = %q after validate, want empty", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "" {
		t.Errorf("shell = %q after validate, want empty", cfg.Defaults.Shell)
	}
	if cfg.Defaults.SyncTimeout != 0 {
		t.Errorf("sync_timeout = %d after validate, want 0", cfg.Defaults.SyncTimeout)
	}
	if cfg.Defaults.RPCPort != 0 {
		t.Errorf("rpc_port = %d after validate, want 0", cfg.Defaults.RPCPort)
	}
}

func TestApplyGlobalDefaults(t *testing.T) {
	cfg := &GlobalConfig{
		Server: ServerConfig{Host: "example.com", User: "deploy"},
	}

	applyGlobalDefaults(cfg)

	if cfg.Server.Port != 22 {
		t.Errorf("port = %d, want 22", cfg.Server.Port)
	}
	if cfg.Defaults.Image != "fusebox/claude:latest" {
		t.Errorf("image = %q, want fusebox/claude:latest", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "/bin/bash" {
		t.Errorf("shell = %q, want /bin/bash", cfg.Defaults.Shell)
	}
	if cfg.Defaults.SyncTimeout != 30 {
		t.Errorf("sync_timeout = %d, want 30", cfg.Defaults.SyncTimeout)
	}
	if cfg.Defaults.RPCPort != 7600 {
		t.Errorf("rpc_port = %d, want 7600", cfg.Defaults.RPCPort)
	}
}

func TestApplyGlobalDefaultsPreservesExplicitValues(t *testing.T) {
	cfg := &GlobalConfig{
		Server: ServerConfig{Host: "example.com", User: "deploy", Port: 2222},
		Defaults: DefaultsConfig{
			Image:       "custom:v1",
			Shell:       "/bin/zsh",
			SyncTimeout: 60,
			RPCPort:     8080,
		},
	}

	applyGlobalDefaults(cfg)

	if cfg.Server.Port != 2222 {
		t.Errorf("port = %d, want 2222 (preserved)", cfg.Server.Port)
	}
	if cfg.Defaults.Image != "custom:v1" {
		t.Errorf("image = %q, want custom:v1 (preserved)", cfg.Defaults.Image)
	}
	if cfg.Defaults.Shell != "/bin/zsh" {
		t.Errorf("shell = %q, want /bin/zsh (preserved)", cfg.Defaults.Shell)
	}
	if cfg.Defaults.SyncTimeout != 60 {
		t.Errorf("sync_timeout = %d, want 60 (preserved)", cfg.Defaults.SyncTimeout)
	}
	if cfg.Defaults.RPCPort != 8080 {
		t.Errorf("rpc_port = %d, want 8080 (preserved)", cfg.Defaults.RPCPort)
	}
}

// --- Roundtrip: parse the example files ---

func TestParseExampleFuseboxYAML(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	path := filepath.Join(root, "fusebox.example.yaml")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("fusebox.example.yaml not found")
	}

	cfg, err := LoadProjectConfig(path)
	if err != nil {
		t.Fatalf("failed to parse fusebox.example.yaml: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	if len(cfg.Actions) == 0 {
		t.Error("expected at least one action")
	}
}
