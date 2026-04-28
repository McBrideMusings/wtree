package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/McBrideMusings/wtree/internal/classify"
	"github.com/McBrideMusings/wtree/internal/gh"
	"github.com/McBrideMusings/wtree/internal/gitwt"
	"github.com/McBrideMusings/wtree/internal/setup"
	"github.com/McBrideMusings/wtree/internal/shim"
	"github.com/McBrideMusings/wtree/internal/slug"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <input>",
	Short: "Create a worktree from a branch, issue/PR number, or GitHub URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		return runAdd(c.Context(), args[0])
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(ctx context.Context, input string) error {
	if input == "" {
		return errors.New("input must not be empty")
	}

	repoRoot, err := gitwt.RepoRoot(ctx)
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}

	resolved, err := resolveInput(ctx, input)
	if err != nil {
		return err
	}

	branch := resolved.branch
	if branch == "" {
		switch {
		case gitwt.IsOwnRepo(ctx):
			branch = resolved.slug
		case resolved.kind == classify.KindText && strings.Contains(input, "/"):
			branch = input
		default:
			branch = "pierce/" + resolved.slug
		}
	}

	branchNote := ""
	isExisting := resolved.isExistingBranch
	if !isExisting {
		switch {
		case gitwt.BranchExistsLocal(ctx, branch):
			isExisting = true
			branchNote = "branch already exists locally — will check it out"
		case gitwt.BranchExistsRemote(ctx, branch):
			isExisting = true
			branchNote = "branch already exists on origin — will check it out"
		}
	}

	list, err := gitwt.List(ctx)
	if err != nil {
		return err
	}
	if existing, ok := gitwt.FindByBranch(list, repoRoot, branch); ok {
		fmt.Fprintf(os.Stderr, "Worktree already exists for branch %q: %s\n", branch, existing.Path)
		shim.PrintCD(existing.Path)
		return nil
	}

	baseBranch := ""
	if !isExisting {
		baseBranch = gitwt.DefaultBranch(ctx)
	}

	worktreePath := filepath.Join(repoRoot, ".worktrees", resolved.slug)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  Detected: %s\n", resolved.summary)
	fmt.Fprintf(os.Stderr, "  Worktree: .worktrees/%s\n", resolved.slug)
	fmt.Fprintf(os.Stderr, "  Branch:   %s\n", branch)
	if baseBranch != "" {
		fmt.Fprintf(os.Stderr, "  From:     %s\n", baseBranch)
	}
	if branchNote != "" {
		fmt.Fprintf(os.Stderr, "  Note:     %s\n", branchNote)
	}
	fmt.Fprintln(os.Stderr)

	if !confirm("Continue? (enter/y: yes · esc/n: cancel) ") {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	}
	fmt.Fprintln(os.Stderr, "Creating worktree...")

	if err := gitwt.EnsureGitignore(repoRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, ".worktrees"), 0o755); err != nil {
		return err
	}

	if isExisting {
		if !gitwt.BranchExistsLocal(ctx, branch) {
			if err := gitwt.FetchBranch(ctx, branch); err != nil {
				return fmt.Errorf("failed to fetch branch %q from origin", branch)
			}
		}
		if err := gitwt.AddExisting(ctx, worktreePath, branch); err != nil {
			return err
		}
	} else {
		if err := gitwt.AddNewBranch(ctx, worktreePath, branch, baseBranch); err != nil {
			return err
		}
	}

	if err := setup.CopyConfigs(repoRoot, worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "  (copy-configs warning: %v)\n", err)
	}
	if err := setup.InstallDeps(worktreePath); err != nil {
		fmt.Fprintf(os.Stderr, "  (install-deps warning: %v)\n", err)
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Worktree: %s\n", worktreePath)
	fmt.Fprintf(os.Stderr, "Branch:   %s\n", branch)
	shim.PrintCD(worktreePath)
	return nil
}

type resolved struct {
	kind             classify.Kind
	branch           string // may be empty if caller derives from slug
	slug             string
	summary          string
	isExistingBranch bool
}

func resolveInput(ctx context.Context, input string) (resolved, error) {
	r := classify.Classify(input)
	out := resolved{kind: r.Kind}

	if r.Kind == classify.KindPR || r.Kind == classify.KindIssue {
		current, err := gitwt.OriginNWO(ctx)
		if err != nil {
			return out, fmt.Errorf("could not determine current repo from origin")
		}
		if !strings.EqualFold(r.NWO, current) {
			return out, fmt.Errorf("repo mismatch: URL is for %s but you're in %s", r.NWO, current)
		}
	}

	switch r.Kind {
	case classify.KindPR:
		pr, err := gh.ViewPR(ctx, r.Number)
		if err != nil {
			return out, fmt.Errorf("failed to fetch PR #%d", r.Number)
		}
		out.branch = pr.HeadBranch
		out.slug = slug.Sanitize(pr.HeadBranch)
		out.summary = fmt.Sprintf("PR #%d: %s → branch %q", r.Number, pr.Title, pr.HeadBranch)
		out.isExistingBranch = true
	case classify.KindIssue:
		iss, err := gh.ViewIssue(ctx, r.Number)
		if err != nil {
			return out, fmt.Errorf("failed to fetch issue #%d", r.Number)
		}
		out.slug = slug.IssueSlug(r.Number, iss.Title)
		out.summary = fmt.Sprintf("Issue #%d: %s", r.Number, iss.Title)
	case classify.KindNumber:
		if pr, err := gh.ViewPR(ctx, r.Number); err == nil {
			out.branch = pr.HeadBranch
			out.slug = slug.Sanitize(pr.HeadBranch)
			out.summary = fmt.Sprintf("PR #%d: %s → branch %q", r.Number, pr.Title, pr.HeadBranch)
			out.isExistingBranch = true
		} else {
			iss, err := gh.ViewIssue(ctx, r.Number)
			if err != nil {
				return out, fmt.Errorf("#%d is not a PR or issue in this repo", r.Number)
			}
			out.slug = slug.IssueSlug(r.Number, iss.Title)
			out.summary = fmt.Sprintf("Issue #%d: %s", r.Number, iss.Title)
		}
	case classify.KindText:
		switch {
		case gitwt.BranchExistsLocal(ctx, input):
			out.branch = input
			out.slug = slug.Sanitize(input)
			out.summary = fmt.Sprintf("Branch %q (local)", input)
			out.isExistingBranch = true
		case gitwt.BranchExistsRemote(ctx, input):
			out.branch = input
			out.slug = slug.Sanitize(input)
			out.summary = fmt.Sprintf("Branch %q (remote)", input)
			out.isExistingBranch = true
		default:
			out.slug = slug.Sanitize(input)
			out.summary = fmt.Sprintf("New branch %q", input)
		}
	}

	if out.slug == "" {
		out.slug = "wt"
	}
	return out, nil
}
