package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ProjectConfig represents a fusebox.yaml file.
type ProjectConfig struct {
	Version int           `yaml:"version"`
	Sync    SyncConfig    `yaml:"sync"`
	Actions map[string]Action `yaml:"actions"`
}

// SyncConfig holds Mutagen sync ignore patterns.
type SyncConfig struct {
	Ignore []string `yaml:"ignore"`
}

// Action defines a whitelisted local action the agent can trigger.
type Action struct {
	Description string           `yaml:"description"`
	Exec        string           `yaml:"exec"`
	Timeout     int              `yaml:"timeout,omitempty"`
	Params      map[string]Param `yaml:"params,omitempty"`
}

// Param defines validation rules for an action parameter.
type Param struct {
	Type    string   `yaml:"type"`
	Pattern string   `yaml:"pattern,omitempty"`
	Values  []string `yaml:"values,omitempty"`
	Range   []int    `yaml:"range,omitempty"`
}

// LoadProjectConfig reads and parses a fusebox.yaml file.
func LoadProjectConfig(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading project config: %w", err)
	}

	return ParseProjectConfig(data)
}

// ParseProjectConfig parses fusebox.yaml content from bytes.
func ParseProjectConfig(data []byte) (*ProjectConfig, error) {
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing project config: %w", err)
	}

	if err := validateProjectConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
