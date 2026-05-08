package gitwt

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Branch is a local branch eligible for cleanup.
type Branch struct {
	Name       string
	IsMerged   bool
	IsStale    bool
	IsPersonal bool // true if the configured user has at least one commit unique to this branch
	LastCommit time.Time
	AgeStr     string
}

// trimmedGit runs git silently and returns the trimmed stdout.
func trimmedGit(ctx context.Context, args ...string) string {
	out, _ := runGitSilent(ctx, args...)
	return strings.TrimSpace(out)
}

// ListCleanupCandidates returns local branches that are merged into the default
// branch or whose last commit is older than staleAfter. The current branch, the
// default branch, and branches checked out in any worktree are excluded.
//
// Merged status is checked against origin/<default> first; falls back to the
// local default branch if the remote ref is unavailable. IsPersonal is true
// when the configured user.email appears in commits unique to that branch.
func ListCleanupCandidates(ctx context.Context, staleAfter time.Duration) ([]Branch, error) {
	defaultBranch := DefaultBranch(ctx)
	currentBranch := trimmedGit(ctx, "symbolic-ref", "--short", "HEAD")
	authorEmail := trimmedGit(ctx, "config", "user.email")

	worktrees, err := List(ctx)
	if err != nil {
		return nil, err
	}
	worktreeBranches := map[string]bool{}
	for _, wt := range worktrees {
		if wt.Branch != "" {
			worktreeBranches[wt.Branch] = true
		}
	}

	refsOut, err := runGitSilent(ctx, "for-each-ref",
		"--format=%(refname:short)\t%(committerdate:unix)\t%(committerdate:relative)",
		"refs/heads/")
	if err != nil {
		return nil, err
	}

	mergedSet := buildMergedSet(ctx, defaultBranch)
	cutoff := time.Now().Add(-staleAfter)

	var result []Branch
	for _, line := range strings.Split(refsOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		name, tsStr, ageStr := parts[0], parts[1], parts[2]

		if name == "" || name == defaultBranch || name == currentBranch {
			continue
		}
		if worktreeBranches[name] {
			continue
		}

		ts, err := strconv.ParseInt(strings.TrimSpace(tsStr), 10, 64)
		if err != nil {
			continue
		}
		lastCommit := time.Unix(ts, 0)

		merged := mergedSet[name]
		stale := lastCommit.Before(cutoff)
		personal := isPersonalBranch(ctx, name, defaultBranch, authorEmail)

		// Non-personal branches are always candidates regardless of age.
		if !merged && !stale && personal {
			continue
		}

		result = append(result, Branch{
			Name:       name,
			IsMerged:   merged,
			IsStale:    stale,
			IsPersonal: personal,
			LastCommit: lastCommit,
			AgeStr:     strings.TrimSpace(ageStr),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LastCommit.Before(result[j].LastCommit)
	})

	return result, nil
}

// buildMergedSet returns the set of local branches considered merged into the
// default branch. Checks origin/<default> first and falls back to the local
// default branch if the remote ref is unavailable. Returns an empty set when
// defaultBranch is unknown.
func buildMergedSet(ctx context.Context, defaultBranch string) map[string]bool {
	mergedSet := map[string]bool{}
	if defaultBranch == "" {
		return mergedSet
	}

	out, err := runGitSilent(ctx, "branch", "--merged", "origin/"+defaultBranch, "--format=%(refname:short)")
	if err != nil {
		out, _ = runGitSilent(ctx, "branch", "--merged", defaultBranch, "--format=%(refname:short)")
	}
	for _, b := range strings.Split(out, "\n") {
		if b = strings.TrimSpace(b); b != "" {
			mergedSet[b] = true
		}
	}
	return mergedSet
}

// isPersonalBranch reports whether authorEmail has at least one commit reachable
// from branch but not from defaultBranch. Returns false when authorEmail is empty.
func isPersonalBranch(ctx context.Context, branch, defaultBranch, authorEmail string) bool {
	if authorEmail == "" {
		return false
	}
	rangeArg := "refs/heads/" + branch
	if defaultBranch != "" {
		rangeArg = defaultBranch + "..refs/heads/" + branch
	}
	return trimmedGit(ctx, "log", "-1", "--author="+authorEmail, "--format=%H", rangeArg) != ""
}
