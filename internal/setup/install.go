package setup

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/McBrideMusings/wtree/internal/config"
)

// DefaultInstallRecipe reproduces wtree's historical hardcoded behavior: the
// lockfile→tool priority table, the pruned directory set, and `<tool> install`.
// Lockfiles are ordered high→low priority (first present in a dir wins).
func DefaultInstallRecipe() config.InstallRecipe {
	return config.InstallRecipe{
		Command: "{tool} install",
		SkipDirs: []string{
			"node_modules", ".git", ".worktrees", ".next",
			"dist", "build", "target", ".venv", "venv", ".turbo", ".cache",
		},
		Lockfiles: []config.LockfileRule{
			{File: "bun.lockb", Tool: "bun"},
			{File: "bun.lock", Tool: "bun"},
			{File: "pnpm-lock.yaml", Tool: "pnpm"},
			{File: "yarn.lock", Tool: "yarn"},
			{File: "package-lock.json", Tool: "npm"},
		},
	}
}

// installPick is one directory and the command to run in it.
type installPick struct {
	dir string // absolute
	cmd string // {tool}-substituted command
}

// planInstalls walks root and, for each directory, selects the highest-priority
// lockfile present and resolves the command to run there. Pure (no side
// effects) so it can be unit-tested. Directories named in the recipe's SkipDirs
// are not descended into (the root itself is never pruned). Returns picks sorted
// by directory for deterministic output.
func planInstalls(root string, r config.InstallRecipe) []installPick {
	skip := make(map[string]bool, len(r.SkipDirs))
	for _, d := range r.SkipDirs {
		skip[d] = true
	}
	// priority = position in the lockfile list (earlier = higher). Map filename
	// to (tool, priority); lower priorityIdx wins.
	type rule struct {
		tool     string
		priority int
	}
	byFile := make(map[string]rule, len(r.Lockfiles))
	for i, lf := range r.Lockfiles {
		// keep the first occurrence's priority if a file is listed twice
		if _, ok := byFile[lf.File]; !ok {
			byFile[lf.File] = rule{tool: lf.Tool, priority: i}
		}
	}

	bestTool := map[string]string{} // dir -> tool
	bestPrio := map[string]int{}    // dir -> priorityIdx (lower = better)

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skip[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if ru, ok := byFile[d.Name()]; ok {
			dir := filepath.Dir(path)
			if cur, seen := bestPrio[dir]; !seen || ru.priority < cur {
				bestPrio[dir] = ru.priority
				bestTool[dir] = ru.tool
			}
		}
		return nil
	})

	dirs := make([]string, 0, len(bestTool))
	for dir := range bestTool {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	template := r.Command
	if strings.TrimSpace(template) == "" {
		template = "{tool} install"
	}
	picks := make([]installPick, 0, len(dirs))
	for _, dir := range dirs {
		cmd := strings.ReplaceAll(template, "{tool}", bestTool[dir])
		picks = append(picks, installPick{dir: dir, cmd: cmd})
	}
	return picks
}

// InstallDepsWithRecipe runs the install recipe across worktreePath. Honors
// WTREE_SKIP_INSTALL. A failing install in one directory is reported but does
// not stop the others.
func InstallDepsWithRecipe(worktreePath string, r config.InstallRecipe) error {
	if os.Getenv("WTREE_SKIP_INSTALL") != "" {
		fmt.Fprintln(os.Stderr, "Skipping dependency install (WTREE_SKIP_INSTALL set).")
		return nil
	}
	picks := planInstalls(worktreePath, r)
	for _, p := range picks {
		rel, err := filepath.Rel(worktreePath, p.dir)
		if err != nil || rel == "" {
			rel = "."
		}
		fmt.Fprintf(os.Stderr, "Installing dependencies (%s) in %s...\n", p.cmd, rel)
		cmd := exec.Command("sh", "-c", p.cmd)
		cmd.Dir = p.dir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  (install failed in %s)\n", rel)
		}
	}
	return nil
}

// InstallDeps runs the recursive dependency install with the built-in default
// recipe. Retained as the back-compat entry point used when a repo has no
// [commands] configuration at all.
func InstallDeps(worktreePath string) error {
	return InstallDepsWithRecipe(worktreePath, DefaultInstallRecipe())
}
