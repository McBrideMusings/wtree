package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/McBrideMusings/wtree/internal/gitwt"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List all worktrees",
	Args:    cobra.NoArgs,
	RunE: func(c *cobra.Command, args []string) error {
		return runList(c.Context())
	},
}

func init() {
	rootCmd.AddCommand(lsCmd)
}

func runList(ctx context.Context) error {
	list, err := gitwt.List(ctx)
	if err != nil {
		return err
	}
	for _, w := range list {
		branch := w.Branch
		switch {
		case w.Bare:
			branch = "(bare)"
		case w.Detached:
			head := w.Head
			if len(head) > 7 {
				head = head[:7]
			}
			branch = "(detached " + head + ")"
		}
		fmt.Fprintf(os.Stderr, "%s  %s\n", w.Path, branch)
	}
	return nil
}
