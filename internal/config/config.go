package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      Server   `yaml:"server"`
	Claude      Claude   `yaml:"claude"`
	Tmux        Tmux     `yaml:"tmux,omitempty"`
	Sandbox     Sandbox  `yaml:"sandbox,omitempty"`
	BrowseRoots []string `yaml:"browse_roots"`
	ServerPath  string   `yaml:"helper_path"` // yaml tag unchanged for backward compat
}

type Sandbox struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	DataDir string `yaml:"data_dir,omitempty"` // default: ~/.fusebox
}

type Server struct {
	Host    string `yaml:"host"`
	User    string `yaml:"user"`
	HomeDir string `yaml:"home_dir"`
}

type Claude struct {
	Flags  string `yaml:"flags"`
	Teams  bool   `yaml:"teams,omitempty"`
	Model  string `yaml:"model,omitempty"`  // sonnet, opus, haiku
	Effort string `yaml:"effort,omitempty"` // low, medium, high, max
}

type Tmux struct {
	Passthrough bool `yaml:"passthrough,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Claude: Claude{
			Flags: "--dangerously-skip-permissions --remote-control",
		},
	}
}

// Validate checks that required fields are set.
func (c Config) Validate() error {
	if c.Server.Host == "" {
		return fmt.Errorf("server.host is required — see config.example.yaml")
	}
	if c.Server.User == "" {
		return fmt.Errorf("server.user is required — see config.example.yaml")
	}
	return nil
}

// ResolveHomeDir returns the server-side home directory.
func (c Config) ResolveHomeDir() string {
	if c.Server.HomeDir != "" {
		return c.Server.HomeDir
	}
	return "/home/" + c.Server.User
}

// ResolveServerPath returns the server-side path to the fusebox binary.
func (c Config) ResolveServerPath() string {
	if c.ServerPath != "" {
		return c.ServerPath
	}
	return c.ResolveHomeDir() + "/bin/fusebox"
}

// ResolveSandboxDataDir returns the sandbox data directory.
func (c Config) ResolveSandboxDataDir() string {
	if c.Sandbox.DataDir != "" {
		return c.Sandbox.DataDir
	}
	return c.ResolveHomeDir() + "/.fusebox"
}

// SSHTarget returns user@host for mutagen and SSH commands.
func (c Config) SSHTarget() string {
	return c.Server.User + "@" + c.Server.Host
}

func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "fusebox", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "fusebox", "config.yaml")
}

func Save(cfg Config) error {
	return SaveTo(cfg, configPath())
}

func SaveTo(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func Load() (Config, error) {
	return LoadFrom(configPath())
}

func LoadFrom(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
