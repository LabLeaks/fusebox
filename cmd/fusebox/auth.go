package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// globalConfig is a minimal representation of ~/.fusebox/config for token storage.
// Will be replaced by internal/config.GlobalConfig once that package lands.
type globalConfig struct {
	Server   *configServer   `yaml:"server,omitempty"`
	Token    string          `yaml:"token"`
	Defaults *configDefaults `yaml:"defaults,omitempty"`
}

type configServer struct {
	Host string `yaml:"host,omitempty"`
	User string `yaml:"user,omitempty"`
	Port int    `yaml:"port,omitempty"`
}

type configDefaults struct {
	Image       string `yaml:"image,omitempty"`
	Shell       string `yaml:"shell,omitempty"`
	SyncTimeout int    `yaml:"sync_timeout,omitempty"`
	RPCPort     int    `yaml:"rpc_port,omitempty"`
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate and store Claude API token",
	Long: `Runs claude setup-token to capture the auth token and stores it in
~/.fusebox/config for use during fusebox up.`,
	RunE: runAuth,
}

func runAuth(cmd *cobra.Command, args []string) error {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude binary not found in PATH — install Claude Code first: https://docs.anthropic.com/en/docs/claude-code")
	}

	proc := exec.Command(claudeBin, "setup-token")
	proc.Stdin = os.Stdin
	proc.Stderr = os.Stderr

	var stdout bytes.Buffer
	proc.Stdout = &stdout

	if err := proc.Run(); err != nil {
		return fmt.Errorf("claude setup-token failed: %w", err)
	}

	token := strings.TrimSpace(stdout.String())
	if token == "" {
		return fmt.Errorf("claude setup-token returned empty token")
	}

	configPath, err := fuseboxConfigPath()
	if err != nil {
		return err
	}

	cfg, err := loadOrCreateConfig(configPath)
	if err != nil {
		return err
	}

	cfg.Token = token

	if err := writeConfig(configPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Token stored in %s\n", configPath)
	return nil
}

func fuseboxConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".fusebox", "config"), nil
}

func loadOrCreateConfig(path string) (*globalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &globalConfig{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg globalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}

func writeConfig(path string, cfg *globalConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
