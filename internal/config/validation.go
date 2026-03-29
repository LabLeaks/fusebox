package config

import (
	"fmt"
	"regexp"
)

func validateProjectConfig(cfg *ProjectConfig) error {
	if cfg.Version == 0 {
		return fmt.Errorf("project config: version is required")
	}
	if cfg.Version != 1 {
		return fmt.Errorf("project config: unsupported version %d (expected 1)", cfg.Version)
	}

	for name, action := range cfg.Actions {
		if action.Exec == "" {
			return fmt.Errorf("project config: action %q: exec is required", name)
		}
		for paramName, param := range action.Params {
			if err := validateParam(name, paramName, param); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateParam(actionName, paramName string, p Param) error {
	prefix := fmt.Sprintf("project config: action %q param %q", actionName, paramName)

	switch p.Type {
	case "regex":
		if p.Pattern == "" {
			return fmt.Errorf("%s: regex type requires pattern", prefix)
		}
		if _, err := regexp.Compile(p.Pattern); err != nil {
			return fmt.Errorf("%s: invalid regex pattern: %w", prefix, err)
		}
	case "enum":
		if len(p.Values) == 0 {
			return fmt.Errorf("%s: enum type requires values", prefix)
		}
	case "int":
		if len(p.Range) != 2 {
			return fmt.Errorf("%s: int type requires range [min, max]", prefix)
		}
		if p.Range[0] > p.Range[1] {
			return fmt.Errorf("%s: range min (%d) must be <= max (%d)", prefix, p.Range[0], p.Range[1])
		}
	case "":
		return fmt.Errorf("%s: type is required (regex, enum, or int)", prefix)
	default:
		return fmt.Errorf("%s: unknown type %q (expected regex, enum, or int)", prefix, p.Type)
	}

	return nil
}

func validateGlobalConfig(cfg *GlobalConfig) error {
	if cfg.Server.Host == "" {
		return fmt.Errorf("global config: server.host is required")
	}
	if cfg.Server.User == "" {
		return fmt.Errorf("global config: server.user is required")
	}
	return nil
}

func applyGlobalDefaults(cfg *GlobalConfig) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 22
	}
	if cfg.Defaults.Image == "" {
		cfg.Defaults.Image = "fusebox/claude:latest"
	}
	if cfg.Defaults.Shell == "" {
		cfg.Defaults.Shell = "/bin/bash"
	}
	if cfg.Defaults.SyncTimeout == 0 {
		cfg.Defaults.SyncTimeout = 30
	}
	if cfg.Defaults.RPCPort == 0 {
		cfg.Defaults.RPCPort = 7600
	}
}
