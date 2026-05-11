package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type pluginRegistry struct {
	Version int                        `json:"version"`
	Plugins map[string][]pluginInstall `json:"plugins"`
}

type pluginInstall struct {
	Scope        string `json:"scope"`
	ProjectPath  string `json:"projectPath,omitempty"`
	InstallPath  string `json:"installPath"`
	Version      string `json:"version"`
	InstalledAt  string `json:"installedAt"`
	LastUpdated  string `json:"lastUpdated"`
	GitCommitSha string `json:"gitCommitSha,omitempty"`
}

func claudePluginsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), nil
}

// RegisterClaudePlugins copies any local-scoped plugin entries for repoRoot
// into entries for worktreePath in ~/.claude/plugins/installed_plugins.json.
func RegisterClaudePlugins(repoRoot, worktreePath string) error {
	path, err := claudePluginsFile()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var reg pluginRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return err
	}

	added := 0
	for key, entries := range reg.Plugins {
		existing := make(map[string]bool, len(entries))
		for _, e := range entries {
			existing[e.ProjectPath] = true
		}
		if existing[worktreePath] {
			continue
		}
		var toAdd []pluginInstall
		for _, e := range entries {
			if e.Scope == "local" && e.ProjectPath == repoRoot {
				cp := e
				cp.ProjectPath = worktreePath
				toAdd = append(toAdd, cp)
			}
		}
		if len(toAdd) > 0 {
			reg.Plugins[key] = append(entries, toAdd...)
			added += len(toAdd)
		}
	}

	if added == 0 {
		return nil
	}
	out, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "  Registered %d Claude plugin(s) for worktree\n", added)
	return nil
}

// DeregisterClaudePlugins removes plugin entries whose projectPath matches
// worktreePath from ~/.claude/plugins/installed_plugins.json.
func DeregisterClaudePlugins(worktreePath string) error {
	path, err := claudePluginsFile()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var reg pluginRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return err
	}

	removed := 0
	for key, entries := range reg.Plugins {
		var kept []pluginInstall
		for _, e := range entries {
			if e.ProjectPath == worktreePath {
				removed++
			} else {
				kept = append(kept, e)
			}
		}
		reg.Plugins[key] = kept
	}

	if removed == 0 {
		return nil
	}
	out, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "  Removed %d Claude plugin entry/entries for worktree\n", removed)
	return nil
}
