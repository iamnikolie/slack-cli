package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// BaseURL is the Slack Web API root. Slack has no host variants, so it is fixed.
const BaseURL = "https://slack.com/api"

type Config struct {
	Token string `yaml:"token"`
}

// Validate errors when no token is configured.
func (c *Config) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("token not set: run 'slk --config <name> config init' or set SLK_TOKEN")
	}
	return nil
}

// slkHome returns the config dir for a workspace profile.
// SLK_HOME overrides the base (test escape hatch); default base is ~/.slk.
func slkHome(profile string) (string, error) {
	base := os.Getenv("SLK_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".slk")
	}
	if profile != "" {
		return filepath.Join(base, profile), nil
	}
	return base, nil
}

// Dir exposes the resolved profile directory (used by the cache package).
func Dir(profile string) (string, error) { return slkHome(profile) }

// Load reads config from the profile dir. SLK_TOKEN overrides the file value.
func Load(profile string) (*Config, error) {
	cfg := &Config{Token: os.Getenv("SLK_TOKEN")}

	dir, err := slkHome(profile)
	if err != nil {
		return nil, fmt.Errorf("config.Load: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("config.Load: %w", err)
	}
	if err == nil {
		var fileCfg Config
		if e := yaml.Unmarshal(data, &fileCfg); e != nil {
			return nil, fmt.Errorf("config.Load: parse: %w", e)
		}
		if cfg.Token == "" {
			cfg.Token = fileCfg.Token
		}
	}
	return cfg, nil
}

// Save writes the token to ~/.slk/<profile>/config.yaml (dir 0700, file 0600).
func Save(token, profile string) error {
	dir, err := slkHome(profile)
	if err != nil {
		return fmt.Errorf("config.Save: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("config.Save: mkdir: %w", err)
	}
	data, err := yaml.Marshal(Config{Token: token})
	if err != nil {
		return fmt.Errorf("config.Save: marshal: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0600)
}
