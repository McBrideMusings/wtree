package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/McBrideMusings/wtree/internal/config"
	"github.com/McBrideMusings/wtree/internal/gitwt"
	"github.com/McBrideMusings/wtree/internal/picker"
	"github.com/McBrideMusings/wtree/internal/shim"
)

func runPicker(ctx context.Context) error {
	repoRoot, list, current, defaultBranch, err := loadPickerState(ctx, "No worktrees. Use `wtree add <input>` to create one.", true)
	if err != nil || list == nil {
		return err
	}

	prompt := "Worktrees (↑/↓ navigate · enter: cd · x: remove · q: quit):"
	sel, err := picker.Run(ctx, prompt, picker.DefaultEnter, list, current, repoRoot, defaultBranch)
	if err != nil {
		return err
	}
	switch sel.Action {
	case picker.ActionEnter:
		shim.PrintCD(sel.Worktree.Path)
		fmt.Fprintf(os.Stderr, "Now in: %s\n", sel.Worktree.Path)
		return nil
	case picker.ActionRemove:
		return doRemove(ctx, repoRoot, sel.Worktree.Path, false)
	case picker.ActionRemoveMerged:
		return doRemoveBatch(ctx, repoRoot, sel.Worktrees)
	case picker.ActionPull:
		return doPull(ctx, repoRoot, defaultBranch)
	case picker.ActionEditConfig:
		return openConfig(ctx, repoRoot)
	case picker.ActionEditGlobalConfig:
		return openGlobalConfig(ctx)
	default:
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	}
}

func runRemoveViaPicker(ctx context.Context) error {
	repoRoot, filtered, current, _, err := loadPickerState(ctx, "No worktrees to remove.", false)
	if err != nil || filtered == nil {
		return err
	}

	prompt := "Select worktree to remove (↑/↓ navigate · enter: remove · q: quit):"
	sel, err := picker.Run(ctx, prompt, picker.DefaultRemove, filtered, current, "", "")
	if err != nil {
		return err
	}
	switch sel.Action {
	case picker.ActionRemove, picker.ActionEnter:
		return doRemove(ctx, repoRoot, sel.Worktree.Path, false)
	case picker.ActionEditConfig:
		return openConfig(ctx, repoRoot)
	case picker.ActionEditGlobalConfig:
		return openGlobalConfig(ctx)
	default:
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	}
}

// loadPickerState gathers the data both picker entry points need. When
// includeMain is true the primary worktree is pinned as the first entry and the
// repo's default branch is returned (for the behind-origin check); otherwise the
// main worktree is filtered out and defaultBranch is empty. Returns a nil list
// (with nil error) when there are no worktrees to show, after printing emptyMsg
// to stderr — callers should treat that as a no-op exit.
func loadPickerState(ctx context.Context, emptyMsg string, includeMain bool) (repoRoot string, list []gitwt.Worktree, current, defaultBranch string, err error) {
	repoRoot, err = gitwt.RepoRoot(ctx)
	if err != nil {
		return "", nil, "", "", errors.New("not inside a git repository")
	}
	all, err := gitwt.List(ctx)
	if err != nil {
		return "", nil, "", "", err
	}
	current, _ = gitwt.TopLevel(ctx)
	children := filterNonMain(all, repoRoot)

	if !includeMain {
		if len(children) == 0 {
			fmt.Fprintln(os.Stderr, emptyMsg)
			return "", nil, "", "", nil
		}
		return repoRoot, children, current, "", nil
	}

	// Pin the primary worktree to the top of the list.
	ordered := make([]gitwt.Worktree, 0, len(children)+1)
	for _, w := range all {
		if w.Path == repoRoot && !w.Bare {
			ordered = append(ordered, w)
			break
		}
	}
	ordered = append(ordered, children...)
	if len(ordered) == 0 {
		fmt.Fprintln(os.Stderr, emptyMsg)
		return "", nil, "", "", nil
	}
	return repoRoot, ordered, current, gitwt.DefaultBranch(ctx), nil
}

// doPull fast-forwards the primary worktree's default branch from origin. It
// refuses to pull when the primary worktree is checked out on a different
// branch, to avoid merging origin/<default> into an unrelated branch.
func doPull(ctx context.Context, repoRoot, defaultBranch string) error {
	if defaultBranch == "" {
		defaultBranch = gitwt.DefaultBranch(ctx)
	}
	if defaultBranch == "" {
		return errors.New("could not determine the default branch")
	}
	if cur := gitwt.CurrentBranchAt(ctx, repoRoot); cur != defaultBranch {
		fmt.Fprintf(os.Stderr, "Primary worktree is on %q, not %q — skipping pull to avoid merging into the wrong branch.\n", cur, defaultBranch)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Pulling origin/%s into %s…\n", defaultBranch, repoRoot)
	c := exec.CommandContext(ctx, "git", "-C", repoRoot, "pull", "--ff-only", "origin", defaultBranch)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	return c.Run()
}

func openConfig(ctx context.Context, repoRoot string) error {
	return openConfigPath(ctx, filepath.Join(repoRoot, ".wtree", "config.toml"))
}

func openGlobalConfig(ctx context.Context) error {
	path, err := config.GlobalConfigPath()
	if err != nil {
		return err
	}
	return openConfigPath(ctx, path)
}

func openConfigPath(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := config.WriteDefault(path); err != nil {
			return err
		}
	}
	return launchEditor(ctx, path)
}

func launchEditor(ctx context.Context, path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		fmt.Fprintln(os.Stderr, "Set $EDITOR or $VISUAL to edit the config.")
		return nil
	}
	cmd := exec.CommandContext(ctx, editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func filterNonMain(list []gitwt.Worktree, repoRoot string) []gitwt.Worktree {
	out := make([]gitwt.Worktree, 0, len(list))
	for _, w := range list {
		if w.Path == repoRoot {
			continue
		}
		out = append(out, w)
	}
	return out
}
