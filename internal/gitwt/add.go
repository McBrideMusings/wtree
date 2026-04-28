package gitwt

import (
	"context"
	"os"
	"os/exec"
)

// AddExisting checks out an existing branch into a new worktree at path.
func AddExisting(ctx context.Context, path, branch string) error {
	return runWorktree(ctx, "add", path, branch)
}

// AddNewBranch creates a new branch and adds a worktree for it at path.
// base is the commit-ish to branch from; if empty, Git uses the current HEAD.
func AddNewBranch(ctx context.Context, path, branch, base string) error {
	args := []string{"add", "-b", branch, path}
	if base != "" {
		args = append(args, base)
	}
	return runWorktree(ctx, args...)
}

func runWorktree(ctx context.Context, args ...string) error {
	full := append([]string{"worktree"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
