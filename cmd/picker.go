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
	repoRoot, err := gitwt.RepoRoot(ctx)
	if err != nil {
		return errors.New("not inside a git repository")
	}
	list, err := gitwt.List(ctx)
	if err != nil {
		return err
	}
	current, _ := gitwt.TopLevel(ctx)

	filtered := filterNonMain(list, repoRoot)
	if len(filtered) == 0 {
		fmt.Fprintln(os.Stderr, "No worktrees. Use `wtree add <input>` to create one.")
		return nil
	}

	prompt := "Worktrees (↑/↓ navigate · enter: cd · x: remove · q: quit):"
	sel, err := picker.Run(ctx, prompt, picker.DefaultEnter, filtered, current)
	if err != nil {
		return err
	}
	switch sel.Action {
	case picker.ActionEnter:
		shim.PrintCD(sel.Worktree.Path)
		fmt.Fprintf(os.Stderr, "Now in: %s\n", sel.Worktree.Path)
	case picker.ActionRemove:
		return doRemove(ctx, repoRoot, sel.Worktree.Path, false)
	case picker.ActionEditConfig:
		return openConfig(ctx, repoRoot)
	case picker.ActionEditGlobalConfig:
		return openGlobalConfig(ctx)
	default:
		fmt.Fprintln(os.Stderr, "Cancelled.")
	}
	return nil
}

func runRemoveViaPicker(ctx context.Context) error {
	repoRoot, err := gitwt.RepoRoot(ctx)
	if err != nil {
		return errors.New("not inside a git repository")
	}
	list, err := gitwt.List(ctx)
	if err != nil {
		return err
	}
	current, _ := gitwt.TopLevel(ctx)

	filtered := filterNonMain(list, repoRoot)
	if len(filtered) == 0 {
		fmt.Fprintln(os.Stderr, "No worktrees to remove.")
		return nil
	}

	prompt := "Select worktree to remove (↑/↓ navigate · enter: remove · q: quit):"
	sel, err := picker.Run(ctx, prompt, picker.DefaultRemove, filtered, current)
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
	}
	return nil
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
