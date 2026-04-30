// Package setup runs the post-creation steps after a worktree is added:
// copying env/config files and installing dependencies.
package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/McBrideMusings/wtree/internal/config"
)

// CopyConfigs copies files matching the patterns in .wtree/config.toml (or
// the built-in defaults) from the repo root into the new worktree.
func CopyConfigs(repoRoot, worktreePath string) error {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		return err
	}
	for _, pattern := range cfg.Copy.Patterns {
		matches, err := filepath.Glob(filepath.Join(repoRoot, pattern))
		if err != nil {
			continue
		}
		for _, src := range matches {
			info, err := os.Stat(src)
			if err != nil || info.IsDir() {
				continue
			}
			rel, err := filepath.Rel(repoRoot, src)
			if err != nil {
				continue
			}
			dst := filepath.Join(worktreePath, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "  (failed to create dir for %s: %v)\n", rel, err)
				continue
			}
			if err := copyFile(src, dst); err != nil {
				fmt.Fprintf(os.Stderr, "  (failed to copy %s: %v)\n", rel, err)
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
