package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/McBrideMusings/wtree/internal/gitwt"
	"github.com/McBrideMusings/wtree/internal/shim"
	"github.com/spf13/cobra"
)

var (
	rmForce bool
	rmCmd   = &cobra.Command{
		Use:               "rm [name]",
		Aliases:           []string{"remove"},
		Short:             "Remove a worktree",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeWorktreeNames,
		RunE: func(c *cobra.Command, args []string) error {
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			return runRemove(c.Context(), target, rmForce)
		},
	}
)

func init() {
	rmCmd.Flags().BoolVar(&rmForce, "force", false, "Force remove without confirmation")
	rootCmd.AddCommand(rmCmd)
}

func runRemove(ctx context.Context, target string, force bool) error {
	repoRoot, err := gitwt.RepoRoot(ctx)
	if err != nil {
		return errors.New("not inside a git repository")
	}

	explicitTarget := target != ""

	if target == "" {
		inside, err := gitwt.InsideLinkedWorktree(ctx)
		if err != nil {
			return err
		}
		if inside {
			top, err := topLevel(ctx)
			if err != nil {
				return err
			}
			target = top
			fmt.Fprintf(os.Stderr, "Detected worktree: %s\n", target)
			if !force && !confirm("Remove this worktree? (enter/y: yes · esc/n: cancel) ") {
				fmt.Fprintln(os.Stderr, "Cancelled.")
				return nil
			}
		} else {
			return runRemoveViaPicker(ctx)
		}
	}

	if !strings.Contains(target, string(os.PathSeparator)) {
		target = filepath.Join(repoRoot, ".worktrees", target)
	}
	target = filepath.Clean(target)

	if !force && explicitTarget {
		name := filepath.Base(target)
		if !confirm(fmt.Sprintf("Remove worktree %q? (enter/y: yes · esc/n: cancel) ", name)) {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return nil
		}
	}

	return doRemove(ctx, repoRoot, target, force)
}

// doRemove is the shared remove flow used by `rm` and the picker.
func doRemove(ctx context.Context, repoRoot, target string, force bool) error {
	fmt.Fprintf(os.Stderr, "Removing worktree: %s\n", target)

	list, _ := gitwt.List(ctx)
	branch := ""
	if w, ok := gitwt.FindByPath(list, target); ok && !w.Detached {
		branch = w.Branch
	}

	cwd, _ := os.Getwd()
	if cwd == target || strings.HasPrefix(cwd, target+string(os.PathSeparator)) {
		fmt.Fprintf(os.Stderr, "Changing directory to %s\n", repoRoot)
		shim.PrintCD(repoRoot)
	}
	if err := gitwt.Remove(ctx, target, force); err != nil {
		if force {
			return err
		}
		fmt.Fprintln(os.Stderr)
		if !confirm("Removal failed. Force remove? (enter/y: yes · esc/n: cancel) ") {
			return err
		}
		if err := gitwt.Remove(ctx, target, true); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stderr, "Done.")

	if branch != "" && gitwt.BranchExistsLocal(ctx, branch) {
		prNote := mergedPRNote(ctx, branch)
		prompt := fmt.Sprintf("Also delete branch %q%s? (enter/y: yes · esc/n: skip) ", branch, prNote)
		if confirm(prompt) {
			if err := gitwt.DeleteBranch(ctx, branch); err == nil {
				fmt.Fprintf(os.Stderr, "Deleted branch %q.\n", branch)
			} else if confirm("Branch not merged locally. Force-delete anyway? (enter/y: yes · esc/n: skip) ") {
				if err := gitwt.ForceDeleteBranch(ctx, branch); err == nil {
					fmt.Fprintf(os.Stderr, "Force-deleted branch %q.\n", branch)
				}
			}
		}
	}
	return nil
}

func topLevel(ctx context.Context) (string, error) {
	return gitwt.TopLevel(ctx)
}

func mergedPRNote(ctx context.Context, branch string) string {
	if _, err := exec.LookPath("gh"); err != nil {
		return ""
	}
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(tctx, "gh", "pr", "list",
		"--head", branch, "--state", "merged",
		"--json", "number", "--jq", ".[0].number")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	num := strings.TrimSpace(string(out))
	if num == "" {
		return ""
	}
	return fmt.Sprintf(" (merged via PR #%s)", num)
}
