package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const projectConfigFile = "fusebox.yaml"

// ResolvedConfig combines a project config with a resolved server.
type ResolvedConfig struct {
	Project     *ProjectConfig
	ProjectRoot string
	Server      ServerConfig
	Token       string
	Defaults    DefaultsConfig
}

// ResolveOptions controls how config resolution behaves.
type ResolveOptions struct {
	StartDir       string // directory to start searching from (defaults to cwd)
	GlobalPath     string // path to global config (defaults to ~/.fusebox/config)
	ServerOverride string // CLI flag override for server host
}

// Resolve finds fusebox.yaml by walking up from StartDir, loads the global
// config, and returns a merged ResolvedConfig.
func Resolve(opts ResolveOptions) (*ResolvedConfig, error) {
	startDir := opts.StartDir
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
	}

	projectRoot, err := findProjectRoot(startDir)
	if err != nil {
		return nil, err
	}

	projectCfg, err := LoadProjectConfig(filepath.Join(projectRoot, projectConfigFile))
	if err != nil {
		return nil, err
	}

	globalPath := opts.GlobalPath
	if globalPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		globalPath = filepath.Join(home, ".fusebox", "config")
	}

	globalCfg, err := LoadGlobalConfig(globalPath)
	if err != nil {
		return nil, fmt.Errorf("loading global config (%s): %w", globalPath, err)
	}

	server := globalCfg.Server
	if opts.ServerOverride != "" {
		server.Host = opts.ServerOverride
	}

	return &ResolvedConfig{
		Project:     projectCfg,
		ProjectRoot: projectRoot,
		Server:      server,
		Token:       globalCfg.Token,
		Defaults:    globalCfg.Defaults,
	}, nil
}

// findProjectRoot walks up from dir looking for fusebox.yaml.
func findProjectRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}

	for {
		candidate := filepath.Join(dir, projectConfigFile)
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a fusebox project (no %s found)", projectConfigFile)
		}
		dir = parent
	}
}
