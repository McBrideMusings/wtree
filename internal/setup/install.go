package setup

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

type lockfile struct {
	name     string
	tool     string
	priority int
}

var lockfiles = []lockfile{
	{"bun.lockb", "bun", 4},
	{"bun.lock", "bun", 4},
	{"pnpm-lock.yaml", "pnpm", 3},
	{"yarn.lock", "yarn", 2},
	{"package-lock.json", "npm", 1},
}

var pruneDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".worktrees":   true,
	".next":        true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".venv":        true,
	"venv":         true,
	".turbo":       true,
	".cache":       true,
}

// InstallDeps walks worktreePath, picks the highest-priority lockfile in each
// directory, and runs `<tool> install` there. Skips entirely if
// WTREE_SKIP_INSTALL is set.
func InstallDeps(worktreePath string) error {
	if os.Getenv("WTREE_SKIP_INSTALL") != "" {
		fmt.Fprintln(os.Stderr, "Skipping dependency install (WTREE_SKIP_INSTALL set).")
		return nil
	}

	type pick struct{ dir, tool string }
	picks := map[string]pick{} // dir -> (tool, priority)
	priorities := map[string]int{}

	err := filepath.WalkDir(worktreePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if pruneDirs[d.Name()] && path != worktreePath {
				return fs.SkipDir
			}
			return nil
		}
		for _, lf := range lockfiles {
			if d.Name() == lf.name {
				dir := filepath.Dir(path)
				if lf.priority > priorities[dir] {
					priorities[dir] = lf.priority
					picks[dir] = pick{dir: dir, tool: lf.tool}
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(picks) == 0 {
		return nil
	}

	dirs := make([]string, 0, len(picks))
	for d := range picks {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	for _, dir := range dirs {
		p := picks[dir]
		rel, err := filepath.Rel(worktreePath, dir)
		if err != nil || rel == "" {
			rel = "."
		}
		fmt.Fprintf(os.Stderr, "Installing dependencies with %s in %s...\n", p.tool, rel)
		cmd := exec.Command(p.tool, "install")
		cmd.Dir = dir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  (install failed in %s)\n", rel)
		}
	}
	return nil
}
