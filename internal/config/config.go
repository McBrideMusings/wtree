package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Copy     CopyConfig     `toml:"copy"`
	Symlink  SymlinkConfig  `toml:"symlink"`
	Commands CommandsConfig `toml:"commands"`
}

type CopyConfig struct {
	Patterns []string `toml:"patterns"`
}

type SymlinkConfig struct {
	Patterns []string `toml:"patterns"`
}

// CommandsConfig drives post-create automation in new worktrees. PostCreate is
// an ordered list of steps run on `wtree add` (not on `sync`). InstallDeps is an
// optional override for the built-in "install-deps" recipe; when nil, the recipe
// uses DefaultInstallRecipe so behavior is identical to wtree's legacy install.
type CommandsConfig struct {
	PostCreate  []Step         `toml:"post_create"`
	InstallDeps *InstallRecipe `toml:"install-deps"`
}

// Recipes is the set of builtin recipe names a bare post_create string resolves
// to. A bare string not in this set is treated as a shell command.
var Recipes = map[string]bool{
	"install-deps": true,
}

// Step is one post_create entry. In TOML it may be written as a bare string
// (a builtin recipe name, or a shell command) or as a table with options.
type Step struct {
	Recipe   string // non-empty → run the named builtin recipe
	Run      string // shell command (mutually exclusive with Recipe)
	IfExists string // skip unless this path (relative to the worktree root) exists
	Required bool   // failure aborts the add instead of warning
}

// UnmarshalTOML lets post_create hold a heterogeneous array of strings and
// tables. A string is classified as a recipe (if known) or a shell command.
func (s *Step) UnmarshalTOML(v any) error {
	switch val := v.(type) {
	case string:
		if Recipes[val] {
			s.Recipe = val
		} else {
			s.Run = val
		}
		return nil
	case map[string]any:
		if r, ok := val["recipe"].(string); ok {
			s.Recipe = r
		}
		if r, ok := val["run"].(string); ok {
			s.Run = r
		}
		if e, ok := val["if_exists"].(string); ok {
			s.IfExists = e
		}
		if req, ok := val["required"].(bool); ok {
			s.Required = req
		}
		if s.Recipe == "" && s.Run == "" {
			return fmt.Errorf("post_create step has neither 'recipe' nor 'run': %v", val)
		}
		return nil
	default:
		return fmt.Errorf("invalid post_create step (want string or table): %T", v)
	}
}

// key uniquely identifies a step for de-duplication when merging config files.
func (s Step) key() string {
	if s.Recipe != "" {
		return "recipe:" + s.Recipe
	}
	return "run:" + s.IfExists + "\x00" + s.Run
}

// InstallRecipe is the data-driven form of the recursive dependency installer.
// Lockfiles are ordered high→low priority; per directory the first present
// lockfile wins and its tool runs once. Command is a template; {tool} is
// substituted before the command runs in that directory.
type InstallRecipe struct {
	Command   string         `toml:"command"`
	SkipDirs  []string       `toml:"skip_dirs"`
	Lockfiles []LockfileRule `toml:"lockfiles"`
}

// LockfileRule maps a lockfile name to the tool that installs it.
type LockfileRule struct {
	File string `toml:"file"`
	Tool string `toml:"tool"`
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
		Copy:     CopyConfig{Patterns: mergePatterns(global.Copy.Patterns, local.Copy.Patterns)},
		Symlink:  SymlinkConfig{Patterns: mergePatterns(global.Symlink.Patterns, local.Symlink.Patterns)},
		Commands: mergeCommands(global.Commands, local.Commands),
	}
}

// mergeCommands appends local post_create steps after global ones, dropping
// duplicates (same recipe, or same run+if_exists). A local install-deps override
// wins over a global one; otherwise the global override (if any) is kept.
func mergeCommands(global, local CommandsConfig) CommandsConfig {
	seen := make(map[string]bool, len(global.PostCreate)+len(local.PostCreate))
	out := make([]Step, 0, len(global.PostCreate)+len(local.PostCreate))
	appendUnique := func(steps []Step) {
		for _, st := range steps {
			k := st.key()
			if !seen[k] {
				seen[k] = true
				out = append(out, st)
			}
		}
	}
	appendUnique(global.PostCreate)
	appendUnique(local.PostCreate)

	install := global.InstallDeps
	if local.InstallDeps != nil {
		install = local.InstallDeps
	}
	return CommandsConfig{PostCreate: out, InstallDeps: install}
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

// WriteDefault creates a config.toml at path with empty pattern lists and a
// commands section that reproduces wtree's built-in install behavior explicitly.
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

# Commands run in each new worktree's root on ` + "`wtree add`" + ` (in order).
# A bare string is a builtin recipe name or a shell command; a table adds
# options ({ run = "...", if_exists = "<relpath>", required = false }).
[commands]
post_create = ["install-deps"]

# Optional override of the "install-deps" recipe. Omit to use the built-in
# default (identical to wtree's legacy recursive multi-lockfile install).
# [commands.install-deps]
# command   = "{tool} install"
# skip_dirs = ["node_modules", ".git", ".worktrees", ".next", "dist", "build", "target", ".venv", "venv", ".turbo", ".cache"]
# [[commands.install-deps.lockfiles]]   # ordered high -> low priority
# file = "bun.lockb"
# tool = "bun"
`
	return os.WriteFile(path, []byte(content), 0o644)
}
