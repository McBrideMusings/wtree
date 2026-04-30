package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Copy CopyConfig `toml:"copy"`
}

type CopyConfig struct {
	Patterns []string `toml:"patterns"`
}

var defaultPatterns = []string{".env*", ".dev.vars", ".claude/settings.local.json"}

// Load reads .wtree/config.toml from repoRoot. Returns defaults if the file is absent.
func Load(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, ".wtree", "config.toml")
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return defaults(), nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// WriteDefault creates .wtree/config.toml at path with the default pattern list.
func WriteDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := "[copy]\npatterns = [\n  \".env*\",\n  \".dev.vars\",\n  \".claude/settings.local.json\",\n]\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func defaults() *Config {
	return &Config{Copy: CopyConfig{Patterns: defaultPatterns}}
}
