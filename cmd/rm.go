package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
			top, err := gitwt.TopLevel(ctx)
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
	return nil
}

func doRemoveBatch(ctx context.Context, repoRoot string, worktrees []gitwt.Worktree) error {
	cwd, _ := os.Getwd()
	needsCD := false
	for _, w := range worktrees {
		if cwd == w.Path || strings.HasPrefix(cwd, w.Path+string(os.PathSeparator)) {
			needsCD = true
			break
		}
	}
	var errs []string
	removed := 0
	for _, w := range worktrees {
		name := filepath.Base(w.Path)
		fmt.Fprintf(os.Stderr, "Removing %s ...\n", name)
		if err := gitwt.Remove(ctx, w.Path, false); err != nil {
			if err2 := gitwt.Remove(ctx, w.Path, true); err2 != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", name, err2))
				continue
			}
		}
		fmt.Fprintf(os.Stderr, "  done\n")
		removed++
	}
	if needsCD && removed > 0 {
		fmt.Fprintf(os.Stderr, "Changing directory to %s\n", repoRoot)
		shim.PrintCD(repoRoot)
	}
	if len(errs) > 0 {
		return fmt.Errorf("some removals failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}
