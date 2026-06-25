package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/McBrideMusings/wtree/internal/branchpicker"
	"github.com/McBrideMusings/wtree/internal/gh"
	"github.com/McBrideMusings/wtree/internal/gitwt"
)

var (
	branchesPruneOnly bool
	branchesDryRun    bool
)

var branchesCmd = &cobra.Command{
	Use:   "branches",
	Short: "clean up dead and stale local branches",
	Long: `Finds local branches that are provably dead — a merged or closed PR, an
upstream branch deleted on origin, or work already fully on origin — lists them,
and force-deletes them after you confirm. Anything that survives is offered in a
picker.

Only local branches are ever deleted; origin is never touched.

  wtree branches            list dead branches, confirm, then pick from the rest
  wtree branches --prune    delete dead branches with no prompt (for scripts)
  wtree branches --dry-run  show what would be deleted, delete nothing`,
	RunE: func(c *cobra.Command, args []string) error {
		return runBranches(c.Context())
	},
}

func init() {
	branchesCmd.Flags().BoolVar(&branchesPruneOnly, "prune", false, "delete dead branches only; skip the interactive picker")
	branchesCmd.Flags().BoolVar(&branchesDryRun, "dry-run", false, "show what would be deleted without deleting anything")
	rootCmd.AddCommand(branchesCmd)
}

func runBranches(ctx context.Context) error {
	if _, err := gitwt.RepoRoot(ctx); err != nil {
		return errors.New("not inside a git repository")
	}

	fmt.Fprintln(os.Stderr, "Fetching and pruning remote-tracking branches...")
	gitwt.FetchPrune(ctx)

	fmt.Fprintln(os.Stderr, "Loading branches...")
	candidates, err := gitwt.ListCleanupCandidates(ctx, 4*24*time.Hour)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "No dead or stale branches found.")
		return nil
	}

	// Branches whose PR was squash- or rebase-merged on GitHub never show as
	// merged to git. Best-effort: a gh failure leaves the set empty and we fall
	// back to the git-only signals (local-merged, upstream-gone, fully-pushed).
	mergedPRs, _ := gh.MergedPRHeadBranches(ctx, 200)
	dead, survivors := gitwt.ClassifyDead(candidates, mergedPRs)

	if branchesDryRun {
		reportDryRun(dead, survivors)
		return nil
	}

	if len(dead) > 0 {
		listDead(dead)
		// --prune is the scriptable, no-prompt path; the default lists what it
		// will remove and waits for a y/n first.
		if branchesPruneOnly || confirm(fmt.Sprintf("Delete %d dead %s? (enter/y: yes · esc/n: skip) ", len(dead), plural(len(dead)))) {
			n := deleteDead(ctx, dead)
			fmt.Fprintf(os.Stderr, "Deleted %d dead %s.\n", n, plural(n))
		} else {
			fmt.Fprintln(os.Stderr, "Skipped dead branches.")
		}
	}

	if branchesPruneOnly || len(survivors) == 0 {
		return nil
	}

	picked, err := branchpicker.Run(ctx, survivors)
	if err != nil {
		return err
	}
	if len(picked) == 0 {
		return nil
	}
	deleted := 0
	for _, br := range picked {
		if err := gitwt.ForceDeleteBranch(ctx, br.Name); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete %s: %v\n", br.Name, err)
			continue
		}
		deleted++
	}
	fmt.Fprintf(os.Stderr, "Deleted %d %s.\n", deleted, plural(deleted))
	return nil
}

// listDead prints the dead branches that are about to be deleted, one per line
// with the reason each qualified.
func listDead(dead []gitwt.DeadBranch) {
	fmt.Fprintf(os.Stderr, "%d dead %s to delete (local only — origin untouched):\n", len(dead), plural(len(dead)))
	for _, br := range dead {
		fmt.Fprintf(os.Stderr, "  ✗ %s (%s)\n", br.Name, br.Reason)
	}
}

// deleteDead force-deletes every dead branch and returns how many were removed.
func deleteDead(ctx context.Context, dead []gitwt.DeadBranch) int {
	deleted := 0
	for _, br := range dead {
		if err := gitwt.ForceDeleteBranch(ctx, br.Name); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete %s: %v\n", br.Name, err)
			continue
		}
		deleted++
	}
	return deleted
}

func reportDryRun(dead []gitwt.DeadBranch, survivors []gitwt.Branch) {
	if len(dead) > 0 {
		fmt.Fprintf(os.Stderr, "Would delete %d dead %s:\n", len(dead), plural(len(dead)))
		for _, br := range dead {
			fmt.Fprintf(os.Stderr, "  ✗ %s (%s)\n", br.Name, br.Reason)
		}
	} else {
		fmt.Fprintln(os.Stderr, "No dead branches.")
	}
	if len(survivors) > 0 {
		fmt.Fprintf(os.Stderr, "%d stale %s would be offered in the picker:\n", len(survivors), plural(len(survivors)))
		for _, br := range survivors {
			fmt.Fprintf(os.Stderr, "  · %s (%s)\n", br.Name, br.AgeStr)
		}
	}
}

func plural(n int) string {
	if n == 1 {
		return "branch"
	}
	return "branches"
}
