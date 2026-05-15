package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/McBrideMusings/wtree/internal/gitwt"
	"github.com/McBrideMusings/wtree/internal/setup"
	"github.com/spf13/cobra"
)

var (
	syncDryRun bool
	syncYes    bool
	syncCmd    = &cobra.Command{
		Use:   "sync [name]",
		Short: "Re-copy configured ignored files from the primary repo into worktrees",
		Long: `sync re-copies the files matching the [copy] patterns from the primary
repo root into every child worktree (or just the named one), so .env files,
.claude/settings.local.json, and any other configured paths stay aligned with
the source of truth in the primary repo.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeWorktreeNames,
		RunE: func(c *cobra.Command, args []string) error {
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			return runSync(c.Context(), target, syncDryRun, syncYes)
		},
	}
)

func init() {
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Print the change preview and exit without writing")
	syncCmd.Flags().BoolVarP(&syncYes, "yes", "y", false, "Skip the confirmation prompt")
	rootCmd.AddCommand(syncCmd)
}

type syncTarget struct {
	wt      gitwt.Worktree
	changes []setup.Change
}

func runSync(ctx context.Context, target string, dryRun, yes bool) error {
	repoRoot, err := gitwt.RepoRoot(ctx)
	if err != nil {
		return errors.New("not inside a git repository")
	}
	all, err := gitwt.List(ctx)
	if err != nil {
		return err
	}
	worktrees, err := selectSyncTargets(all, repoRoot, target)
	if err != nil {
		return err
	}
	if len(worktrees) == 0 {
		fmt.Fprintln(os.Stderr, "No child worktrees to sync.")
		return nil
	}

	plans := make([]syncTarget, 0, len(worktrees))
	totalWrites := 0
	for _, w := range worktrees {
		changes, err := setup.Plan(repoRoot, w.Path)
		if err != nil {
			return fmt.Errorf("plan %s: %w", filepath.Base(w.Path), err)
		}
		plans = append(plans, syncTarget{wt: w, changes: changes})
		for _, c := range changes {
			if c.Kind != setup.ChangeIdentical {
				totalWrites++
			}
		}
	}

	printSyncPreview(plans)

	if totalWrites == 0 {
		fmt.Fprintln(os.Stderr, "Everything is already in sync.")
		return nil
	}
	if dryRun {
		return nil
	}
	prompt := fmt.Sprintf("Apply %d change(s) across %d worktree(s)? (enter/y: yes · esc/n: cancel) ", totalWrites, len(plans))
	if !yes && !confirm(prompt) {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	}
	for _, p := range plans {
		written, err := setup.Apply(p.changes)
		if err != nil {
			return err
		}
		if written > 0 {
			fmt.Fprintf(os.Stderr, "%s: wrote %d file(s)\n", filepath.Base(p.wt.Path), written)
		}
	}
	fmt.Fprintln(os.Stderr, "Done.")
	return nil
}

func selectSyncTargets(all []gitwt.Worktree, repoRoot, target string) ([]gitwt.Worktree, error) {
	if target == "" {
		var out []gitwt.Worktree
		for _, w := range all {
			if w.Bare || w.IsMain(repoRoot) {
				continue
			}
			out = append(out, w)
		}
		return out, nil
	}
	path := target
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, ".worktrees", target)
	}
	path = filepath.Clean(path)
	for _, w := range all {
		if w.Path == path {
			if w.Bare || w.IsMain(repoRoot) {
				return nil, fmt.Errorf("cannot sync into the primary or bare worktree")
			}
			return []gitwt.Worktree{w}, nil
		}
	}
	return nil, fmt.Errorf("no worktree found at %s", path)
}

func printSyncPreview(plans []syncTarget) {
	fmt.Fprintln(os.Stderr)
	for _, p := range plans {
		fmt.Fprintf(os.Stderr, "%s\n", filepath.Base(p.wt.Path))
		if len(p.changes) == 0 {
			fmt.Fprintln(os.Stderr, "  (no matching source files)")
			continue
		}
		sorted := append([]setup.Change(nil), p.changes...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Rel < sorted[j].Rel })
		for _, c := range sorted {
			fmt.Fprintf(os.Stderr, "  %s\n", formatChange(c))
		}
	}
	fmt.Fprintln(os.Stderr)
}

func formatChange(c setup.Change) string {
	switch c.Kind {
	case setup.ChangeNew:
		return fmt.Sprintf("+ %s  (new)", c.Rel)
	case setup.ChangeOverwrite:
		return fmt.Sprintf("~ %s  (overwrite, differs)", c.Rel)
	case setup.ChangeIdentical:
		return fmt.Sprintf("= %s  (identical)", c.Rel)
	default:
		return c.Rel
	}
}
