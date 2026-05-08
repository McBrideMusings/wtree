package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/McBrideMusings/wtree/internal/branchpicker"
	"github.com/McBrideMusings/wtree/internal/gitwt"
)

var branchesCmd = &cobra.Command{
	Use:   "branches",
	Short: "clean up stale and merged local branches",
	Long: `Lists local branches that are merged into the default branch or have not
been committed to in more than 4 days, then lets you pick which ones to delete.
Only local branches are deleted — the remote is never touched.`,
	RunE: func(c *cobra.Command, args []string) error {
		return runBranches(c.Context())
	},
}

func init() {
	rootCmd.AddCommand(branchesCmd)
}

func runBranches(ctx context.Context) error {
	if _, err := gitwt.RepoRoot(ctx); err != nil {
		return errors.New("not inside a git repository")
	}

	fmt.Fprintln(os.Stderr, "Loading branches...")
	candidates, err := gitwt.ListCleanupCandidates(ctx, 4*24*time.Hour)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "No stale or merged branches found.")
		return nil
	}

	// Merged branches and non-personal stale branches go to a batch confirm —
	// no individual selection needed. Personal stale branches go to the picker.
	var autoBatch, personalStale []gitwt.Branch
	for _, br := range candidates {
		if br.IsMerged || !br.IsPersonal {
			autoBatch = append(autoBatch, br)
		} else {
			personalStale = append(personalStale, br)
		}
	}

	var toDelete []gitwt.Branch

	if len(autoBatch) > 0 {
		ok, err := branchpicker.RunConfirm(ctx, autoBatch)
		if err != nil {
			return err
		}
		if ok {
			toDelete = append(toDelete, autoBatch...)
		}
	}

	if len(personalStale) > 0 {
		picked, err := branchpicker.Run(ctx, personalStale)
		if err != nil {
			return err
		}
		toDelete = append(toDelete, picked...)
	}

	if len(toDelete) == 0 {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	}

	for _, br := range toDelete {
		if err := gitwt.ForceDeleteBranch(ctx, br.Name); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete %s: %v\n", br.Name, err)
		}
	}

	noun := "branch"
	if len(toDelete) != 1 {
		noun = "branches"
	}
	fmt.Fprintf(os.Stderr, "Deleted %d %s.\n", len(toDelete), noun)
	return nil
}
