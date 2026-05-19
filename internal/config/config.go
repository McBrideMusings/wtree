package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Copy    CopyConfig    `toml:"copy"`
	Symlink SymlinkConfig `toml:"symlink"`
}

type CopyConfig struct {
	Patterns []string `toml:"patterns"`
}

type SymlinkConfig struct {
	Patterns []string `toml:"patterns"`
}

// GlobalConfigPath returns ~/.config/wtree/config.toml.
func GlobalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wtree", "config.toml"), nil
}

// Load reads config from the global path and the repo-local .wtree/config.toml,
// merging their pattern lists. Global patterns form the baseline; local patterns
// are appended (duplicates removed). Returns an empty Config when neither file exists.
func Load(repoRoot string) (*Config, error) {
	globalPath, err := GlobalConfigPath()
	if err != nil {
		return nil, err
	}
	global, err := loadFile(globalPath)
	if err != nil {
		return nil, err
	}
	local, err := loadFile(filepath.Join(repoRoot, ".wtree", "config.toml"))
	if err != nil {
		return nil, err
	}
	return merge(global, local), nil
}

func loadFile(path string) (*Config, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func merge(global, local *Config) *Config {
	if global == nil && local == nil {
		return &Config{}
	}
	if global == nil {
		return local
	}
	if local == nil {
		return global
	}
	return &Config{
		Copy:    CopyConfig{Patterns: mergePatterns(global.Copy.Patterns, local.Copy.Patterns)},
		Symlink: SymlinkConfig{Patterns: mergePatterns(global.Symlink.Patterns, local.Symlink.Patterns)},
	}
}

func mergePatterns(base, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	appendUnique := func(patterns []string) {
		for _, p := range patterns {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	appendUnique(base)
	appendUnique(extra)
	return out
}

// WriteDefault creates a config.toml at path with empty pattern lists for both sections.
func WriteDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := `# Files to symlink into new worktrees (live link back to primary).
[symlink]
patterns = []

# Files to copy into new worktrees (independent per-worktree).
[copy]
patterns = []
`
	return os.WriteFile(path, []byte(content), 0o644)
}
