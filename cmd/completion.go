package cmd

import (
	"path/filepath"

	"github.com/McBrideMusings/wtree/internal/gitwt"
	"github.com/spf13/cobra"
)

// completeWorktreeNames returns the basenames of all linked worktrees so
// `wtree rm <TAB>` shows the same names users see in `wtree ls`.
func completeWorktreeNames(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	ctx := c.Context()
	repoRoot, err := gitwt.RepoRoot(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	list, err := gitwt.List(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(list))
	for _, w := range list {
		if w.Path == repoRoot || w.Bare {
			continue
		}
		names = append(names, filepath.Base(w.Path))
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
