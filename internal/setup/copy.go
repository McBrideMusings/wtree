// Package setup runs the post-creation steps after a worktree is added:
// copying env/config files and installing dependencies.
package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyConfigs mirrors the bash script's behavior: copy every .env* file at
// the repo root into the new worktree, plus .claude/settings.local.json if
// it exists. Failures on individual files are reported to stderr but don't
// abort the rest.
func CopyConfigs(repoRoot, worktreePath string) error {
	matches, err := filepath.Glob(filepath.Join(repoRoot, ".env*"))
	if err != nil {
		return err
	}
	for _, src := range matches {
		info, err := os.Stat(src)
		if err != nil || info.IsDir() {
			continue
		}
		dst := filepath.Join(worktreePath, filepath.Base(src))
		if err := copyFile(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "  (failed to copy %s: %v)\n", filepath.Base(src), err)
		}
	}

	claudeSrc := filepath.Join(repoRoot, ".claude", "settings.local.json")
	if _, err := os.Stat(claudeSrc); err == nil {
		claudeDst := filepath.Join(worktreePath, ".claude")
		if err := os.MkdirAll(claudeDst, 0o755); err != nil {
			return err
		}
		if err := copyFile(claudeSrc, filepath.Join(claudeDst, "settings.local.json")); err != nil {
			fmt.Fprintf(os.Stderr, "  (failed to copy .claude/settings.local.json: %v)\n", err)
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
