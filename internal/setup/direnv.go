package setup

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// DirenvAllow runs `direnv allow` in worktreePath if:
//  1. .envrc exists there, AND
//  2. its SHA-256 matches the .envrc at repoRoot (i.e. it's the file we just
//     copied, not something that changed out from under us)
//
// If direnv is not on PATH the call is silently skipped.
func DirenvAllow(repoRoot, worktreePath string) {
	if _, err := exec.LookPath("direnv"); err != nil {
		return
	}

	dest := filepath.Join(worktreePath, ".envrc")
	if _, err := os.Stat(dest); err != nil {
		return
	}

	src := filepath.Join(repoRoot, ".envrc")
	if _, err := os.Stat(src); err != nil {
		return
	}
	srcHash, err := sha256File(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  (direnv: could not hash source .envrc, skipping allow)\n")
		return
	}
	destHash, err := sha256File(dest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  (direnv: could not hash worktree .envrc, skipping allow)\n")
		return
	}

	if srcHash != destHash {
		fmt.Fprintf(os.Stderr, "  (direnv: .envrc differs from repo root — run 'direnv allow' manually if you trust it)\n")
		return
	}

	cmd := exec.Command("direnv", "allow", ".")
	cmd.Dir = worktreePath
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  (direnv allow failed: %v)\n", err)
	}
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
