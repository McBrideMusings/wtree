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
	Ahead       bool // true if the local tip is ahead of its upstream (has unpushed local-only commits)
	LastCommit  time.Time
	AgeStr      string
}

// DeadBranch is a cleanup candidate classified as safe to force-delete locally,
// with the reason it qualified.
type DeadBranch struct {
	Name   string
	Reason string
}

// ClassifyDead splits cleanup candidates into dead branches — provably safe to
// force-delete locally, origin untouched — and survivors that still need human
// judgment. mergedPRs is the set of head branch names whose PR merged on GitHub
// (from gh.MergedPRHeadBranches); pass an empty map when gh is unavailable.
//
// The merged-PR-by-name signal is gated on the branch not being ahead of its
// upstream: a freshly re-created local branch that reuses a previously merged
// branch's name carries new local-only commits, so it must not be deleted just
// because the old name's PR merged. This is the single classifier both
// `wtree branches` and the dashboard call, so the two can never drift.
func ClassifyDead(cands []Branch, mergedPRs map[string]bool) (dead []DeadBranch, survivors []Branch) {
	for _, br := range cands {
		switch {
		case br.RemoteGone:
			dead = append(dead, DeadBranch{br.Name, "remote gone"})
		case mergedPRs[br.Name] && !br.Ahead:
			dead = append(dead, DeadBranch{br.Name, "PR merged"})
		case br.IsMerged:
			dead = append(dead, DeadBranch{br.Name, "merged"})
		case br.FullyPushed:
			dead = append(dead, DeadBranch{br.Name, "on origin"})
		default:
			survivors = append(survivors, br)
		}
	}
	return dead, survivors
}

// FetchPrune fetches origin and prunes stale remote-tracking refs, so both the
// "remote gone" and "fully pushed" (not-ahead) signals are judged against the
// current state of origin rather than a stale local snapshot. It never mutates
// origin. Errors are swallowed — a repo with no origin, or no network, simply
// classifies against whatever refs are already known locally.
func FetchPrune(ctx context.Context) {
	_, _ = runGitSilent(ctx, "fetch", "--prune", "origin")
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
		ahead := strings.Contains(track, "ahead")
		// Fully pushed: the branch tracks an origin branch and the local tip is
		// not ahead of it, so every local commit already exists on origin.
		// Deleting the local ref loses nothing — origin still has the work.
		// (Accurate only against current origin, so callers FetchPrune first.)
		fullyPushed := upstream != "" && !gone && !ahead

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
			Ahead:       ahead,
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
