// Package gitwt wraps `git worktree` and a few related plumbing commands.
package gitwt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotInRepo is returned when a command is run outside any git repository.
var ErrNotInRepo = errors.New("not inside a git repository")

func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func runGitSilent(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// TopLevel returns the absolute path of the current worktree's top-level
// directory (the "linked" worktree path when invoked from one, or the main
// repo's working tree when invoked from the main repo).
func TopLevel(ctx context.Context) (string, error) {
	top, err := runGitSilent(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotInRepo
	}
	return top, nil
}

// RepoRoot returns the absolute path to the main repo's top-level directory,
// resolving correctly even when the cwd is inside a linked worktree.
func RepoRoot(ctx context.Context) (string, error) {
	top, err := runGitSilent(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotInRepo
	}
	gitDir, err := runGitSilent(ctx, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return top, nil
	}
	commonDir, err := runGitSilent(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return top, nil
	}
	if gitDir != commonDir {
		return filepath.Dir(commonDir), nil
	}
	return top, nil
}

// InsideLinkedWorktree reports whether the cwd is inside a linked worktree
// (as opposed to the main worktree or outside the repo entirely).
func InsideLinkedWorktree(ctx context.Context) (bool, error) {
	gitDir, err := runGitSilent(ctx, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return false, ErrNotInRepo
	}
	commonDir, err := runGitSilent(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return false, err
	}
	return gitDir != commonDir, nil
}

// OriginNWO returns "owner/repo" parsed from the origin remote URL. Supports
// SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo).
func OriginNWO(ctx context.Context) (string, error) {
	url, err := runGitSilent(ctx, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("could not determine origin remote URL")
	}
	url = strings.TrimSuffix(url, ".git")
	switch {
	case strings.HasPrefix(url, "git@github.com:"):
		return strings.TrimPrefix(url, "git@github.com:"), nil
	case strings.HasPrefix(url, "https://github.com/"):
		return strings.TrimPrefix(url, "https://github.com/"), nil
	default:
		return "", fmt.Errorf("origin %q is not a recognized GitHub URL", url)
	}
}

// IsOwnRepo reports whether the current repo's origin URL contains
// "McBrideMusings". Repos without an origin are treated as own repos to match
// the bash script's behavior. Issue #15 (no-remote false positive) is preserved
// here intentionally; we'll re-triage it post-port.
func IsOwnRepo(ctx context.Context) bool {
	url, err := runGitSilent(ctx, "remote", "get-url", "origin")
	if err != nil {
		return true
	}
	return strings.Contains(url, "McBrideMusings")
}

// EnsureGitignore appends ".worktrees" to <repoRoot>/.gitignore if not already
// present. Prints a notice to stderr if it adds the line.
func EnsureGitignore(repoRoot string) error {
	path := filepath.Join(repoRoot, ".gitignore")
	body, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(body), "\n") {
		if line == ".worktrees" {
			return nil
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	if _, err := f.WriteString(".worktrees\n"); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Added .worktrees to .gitignore")
	return nil
}

// BranchExistsLocal reports whether refs/heads/<branch> exists.
func BranchExistsLocal(ctx context.Context, branch string) bool {
	_, err := runGitSilent(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// BranchExistsRemote reports whether refs/remotes/origin/<branch> exists.
func BranchExistsRemote(ctx context.Context, branch string) bool {
	_, err := runGitSilent(ctx, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	return err == nil
}

// FetchBranch fetches a single branch from origin.
func FetchBranch(ctx context.Context, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "fetch", "origin", branch)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
