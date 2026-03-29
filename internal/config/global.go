package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// GlobalConfig represents ~/.fusebox/config.
type GlobalConfig struct {
	Server   ServerConfig   `yaml:"server"`
	Token    string         `yaml:"token"`
	Defaults DefaultsConfig `yaml:"defaults"`
}

// ServerConfig holds SSH connection details for a remote server.
type ServerConfig struct {
	Host string `yaml:"host"`
	User string `yaml:"user"`
	Port int    `yaml:"port"`
}

// DefaultsConfig holds default settings for all sessions.
type DefaultsConfig struct {
	Image       string `yaml:"image"`
	Shell       string `yaml:"shell"`
	SyncTimeout int    `yaml:"sync_timeout"`
	RPCPort     int    `yaml:"rpc_port"`
}

// LoadGlobalConfig reads and parses ~/.fusebox/config.
func LoadGlobalConfig(path string) (*GlobalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading global config: %w", err)
	}

	return ParseGlobalConfig(data)
}

// ParseGlobalConfig parses global config content from bytes.
func ParseGlobalConfig(data []byte) (*GlobalConfig, error) {
	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing global config: %w", err)
	}

	if err := validateGlobalConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
