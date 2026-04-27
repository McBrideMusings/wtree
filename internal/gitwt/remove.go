package gitwt

import (
	"context"
	"os"
	"os/exec"
)

// Remove runs `git worktree remove [--force] <path>` and streams output.
func Remove(ctx context.Context, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// DeleteBranch runs `git branch -d <branch>`. Returns nil on success.
func DeleteBranch(ctx context.Context, branch string) error {
	return runGitInherit(ctx, "branch", "-d", branch)
}

// ForceDeleteBranch runs `git branch -D <branch>`.
func ForceDeleteBranch(ctx context.Context, branch string) error {
	return runGitInherit(ctx, "branch", "-D", branch)
}

func runGitInherit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
