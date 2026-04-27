package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wtree",
	Short: "git worktree helper",
	Long: `wtree manages git worktrees under .worktrees/ with optional GitHub
issue/PR integration. Run with no arguments for an interactive picker;
pass any input (branch name, issue/PR number, GitHub URL) to create a
worktree from it.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.ArbitraryArgs,
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runPicker(c.Context())
		}
		return runAdd(c.Context(), args[0])
	},
}

func Execute() error {
	return rootCmd.Execute()
}
