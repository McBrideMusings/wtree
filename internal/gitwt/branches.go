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
	Name        string
	IsMerged    bool
	IsStale     bool
	IsPersonal  bool // true if the configured user has at least one commit unique to this branch
	RemoteGone  bool // true if the branch tracked an origin branch that no longer exists
	FullyPushed bool // true if it tracks an origin branch and is not ahead of it (no local-only commits)
	LastCommit  time.Time
	AgeStr      string
}

// PruneRemoteTracking removes stale remote-tracking refs (origin branches that
// have been deleted upstream) so that RemoteGone detection is accurate. It makes
// a network call but never fetches commits and never mutates origin. Errors are
// swallowed — a repo with no origin, or no network, simply leaves tracking refs
// as-is and gone detection falls back to whatever is already known locally.
func PruneRemoteTracking(ctx context.Context) {
	_, _ = runGitSilent(ctx, "remote", "prune", "origin")
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
		"--format=%(refname:short)\t%(committerdate:unix)\t%(committerdate:relative)\t%(upstream:short)\t%(upstream:track)",
		"refs/heads/")
	if err != nil {
		return nil, err
	}

	mergedSet := buildMergedSet(ctx, defaultBranch)
	cutoff := time.Now().Add(-staleAfter)

	var result []Branch
	for _, line := range strings.Split(refsOut, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Keep empty trailing fields (no upstream / no track), so split on the
		// raw line, not a trimmed one.
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 3 {
			continue
		}
		name, tsStr, ageStr := parts[0], parts[1], parts[2]
		upstream, track := "", ""
		if len(parts) >= 4 {
			upstream = strings.TrimSpace(parts[3])
		}
		if len(parts) >= 5 {
			track = parts[4]
		}

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
		gone := strings.Contains(track, "gone")
		// Fully pushed: the branch tracks an origin branch and the local tip is
		// not ahead of it, so every local commit already exists on origin.
		// Deleting the local ref loses nothing — origin still has the work.
		fullyPushed := upstream != "" && !gone && !strings.Contains(track, "ahead")

		// Gone and fully-pushed branches are provably safe to drop and are
		// always candidates. Otherwise a fresh personal branch (not merged, not
		// stale, with local-only work) is still in active use and is skipped.
		if !gone && !fullyPushed && !merged && !stale && personal {
			continue
		}

		result = append(result, Branch{
			Name:        name,
			IsMerged:    merged,
			IsStale:     stale,
			IsPersonal:  personal,
			RemoteGone:  gone,
			FullyPushed: fullyPushed,
			LastCommit:  lastCommit,
			AgeStr:      strings.TrimSpace(ageStr),
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
