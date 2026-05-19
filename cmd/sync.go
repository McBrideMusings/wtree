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
		Short: "Sync configured files from the primary repo into worktrees",
		Long: `sync applies the [symlink] and [copy] patterns from the primary repo root
into every child worktree (or just the named one). Symlinked entries are
created or repaired; copied entries are written when missing or changed.`,
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
	wt             gitwt.Worktree
	changes        []setup.Change
	symlinkChanges []setup.SymlinkChange
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
		changes, schanges, err := setup.PlanAll(repoRoot, w.Path)
		if err != nil {
			return fmt.Errorf("plan %s: %w", filepath.Base(w.Path), err)
		}
		plans = append(plans, syncTarget{wt: w, changes: changes, symlinkChanges: schanges})
		for _, c := range changes {
			if c.Kind != setup.ChangeIdentical {
				totalWrites++
			}
		}
		for _, c := range schanges {
			if c.Kind == setup.SymlinkNew || c.Kind == setup.SymlinkWrong {
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
		linked, err := setup.ApplySymlinks(p.symlinkChanges)
		if err != nil {
			return err
		}
		if written > 0 || linked > 0 {
			fmt.Fprintf(os.Stderr, "%s: wrote %d file(s), linked %d\n", filepath.Base(p.wt.Path), written, linked)
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
		if len(p.changes) == 0 && len(p.symlinkChanges) == 0 {
			fmt.Fprintln(os.Stderr, "  (no configured patterns matched)")
			continue
		}
		sorted := append([]setup.Change(nil), p.changes...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Rel < sorted[j].Rel })
		for _, c := range sorted {
			fmt.Fprintf(os.Stderr, "  %s\n", formatChange(c))
		}
		ssorted := append([]setup.SymlinkChange(nil), p.symlinkChanges...)
		sort.Slice(ssorted, func(i, j int) bool { return ssorted[i].Rel < ssorted[j].Rel })
		for _, c := range ssorted {
			fmt.Fprintf(os.Stderr, "  %s\n", formatSymlinkChange(c))
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

func formatSymlinkChange(c setup.SymlinkChange) string {
	switch c.Kind {
	case setup.SymlinkNew:
		return fmt.Sprintf("L %s  (new symlink)", c.Rel)
	case setup.SymlinkExists:
		return fmt.Sprintf("= %s  (symlink ok)", c.Rel)
	case setup.SymlinkWrong:
		return fmt.Sprintf("~ %s  (wrong target, will re-link)", c.Rel)
	case setup.SymlinkConflict:
		return fmt.Sprintf("! %s  (conflict: real file, skip)", c.Rel)
	default:
		return c.Rel
	}
}
