// Package gh wraps the gh CLI for the small number of lookups wtree needs.
package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PRInfo holds the number and state of a pull request.
type PRInfo struct {
	Number int
	State  string // "OPEN", "MERGED", "CLOSED"
}

// PRForBranch returns the most recent PR (any state) whose head branch matches.
// Returns PRInfo{}, false, nil when no matching PR exists.
func PRForBranch(ctx context.Context, branch string) (PRInfo, bool, error) {
	out, err := run(ctx, "pr", "list", "--head", branch, "--state", "all", "--limit", "1", "--json", "number,state", "-q", `.[0] | if . == null then "" else "\(.number)\t\(.state)" end`)
	if err != nil {
		return PRInfo{}, false, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return PRInfo{}, false, nil
	}
	parts := strings.SplitN(out, "\t", 2)
	if len(parts) != 2 {
		return PRInfo{}, false, fmt.Errorf("unexpected gh pr list output: %q", out)
	}
	num, err := strconv.Atoi(parts[0])
	if err != nil {
		return PRInfo{}, false, err
	}
	return PRInfo{Number: num, State: parts[1]}, true, nil
}

// PR contains the head branch and title of a pull request.
type PR struct {
	HeadBranch string
	Title      string
}

// Issue contains the title of an issue.
type Issue struct {
	Title string
}

// ViewPR fetches a PR's headRefName and title. Returns an error if gh fails
// (e.g. number is not a PR).
func ViewPR(ctx context.Context, num int) (PR, error) {
	out, err := run(ctx, "pr", "view", strconv.Itoa(num), "--json", "headRefName,title", "-q", `.headRefName + "\t" + .title`)
	if err != nil {
		return PR{}, err
	}
	parts := strings.SplitN(strings.TrimRight(out, "\n"), "\t", 2)
	if len(parts) != 2 {
		return PR{}, fmt.Errorf("unexpected gh pr view output: %q", out)
	}
	return PR{HeadBranch: parts[0], Title: parts[1]}, nil
}

// ViewIssue fetches an issue's title. Returns an error if gh fails.
func ViewIssue(ctx context.Context, num int) (Issue, error) {
	out, err := run(ctx, "issue", "view", strconv.Itoa(num), "--json", "title", "-q", ".title")
	if err != nil {
		return Issue{}, err
	}
	return Issue{Title: strings.TrimRight(out, "\n")}, nil
}

// RepoURL returns the GitHub web URL for an "owner/repo" string.
func RepoURL(nwo string) string {
	if nwo == "" {
		return ""
	}
	return "https://github.com/" + nwo
}

// PRURL returns the web URL for PR #n in nwo.
func PRURL(nwo string, n int) string {
	if nwo == "" || n <= 0 {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/pull/%d", nwo, n)
}

// IssueURL returns the web URL for issue #n in nwo.
func IssueURL(nwo string, n int) string {
	if nwo == "" || n <= 0 {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/issues/%d", nwo, n)
}

// LinkedIssue returns the first issue number a PR closes, via the PR's
// closingIssuesReferences. Returns (0, false, nil) when the PR closes nothing.
func LinkedIssue(ctx context.Context, prNumber int) (int, bool, error) {
	out, err := run(ctx, "pr", "view", strconv.Itoa(prNumber), "--json", "closingIssuesReferences", "-q", `.closingIssuesReferences[0].number // empty`)
	if err != nil {
		return 0, false, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, false, err
	}
	return n, true, nil
}

// IssueExists reports whether issue #n exists in the current repo. A gh failure
// (including "not found") returns false.
func IssueExists(ctx context.Context, n int) bool {
	_, err := run(ctx, "issue", "view", strconv.Itoa(n), "--json", "number")
	return err == nil
}

// ReviewState classifies where the current user stands on a PR they're involved in.
type ReviewState int

const (
	// ReviewPending means classification has not run yet.
	ReviewPending ReviewState = iota
	// NotReviewed means the user has submitted no review.
	NotReviewed
	// UpdatedSinceReview means the user reviewed, but a newer commit landed after.
	UpdatedSinceReview
	// ReviewedCurrent means the user's latest review covers the latest commit.
	ReviewedCurrent
)

// InboxPR is one PR in the "needs my review" inbox.
type InboxPR struct {
	Number     int
	Title      string
	Author     string
	HeadBranch string
	Updated    time.Time
	State      ReviewState
}

// ReviewCandidates returns open PRs in the current repo that involve the user
// and were not authored by them, minus any whose head branch is in localBranches
// (those are represented as worktrees instead). The PRs come back unclassified
// (State == ReviewPending); call ClassifyReview per PR to fill in the state.
// Results are sorted most-recently-updated first.
func ReviewCandidates(ctx context.Context, localBranches []string) ([]InboxPR, error) {
	if _, err := cachedLogin(ctx); err != nil {
		return nil, err // surface auth problems before the search
	}

	out, err := run(ctx, "pr", "list",
		"--search", "is:open involves:@me -author:@me",
		"--limit", "30",
		"--json", "number,title,headRefName,updatedAt,author")
	if err != nil {
		return nil, err
	}
	var listed []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		HeadRefName string `json:"headRefName"`
		UpdatedAt   string `json:"updatedAt"`
		Author      struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		return nil, err
	}

	local := make(map[string]bool, len(localBranches))
	for _, b := range localBranches {
		local[b] = true
	}

	var candidates []InboxPR
	for _, p := range listed {
		if local[p.HeadRefName] {
			continue // already pulled down as a worktree
		}
		updated, _ := time.Parse(time.RFC3339, p.UpdatedAt)
		candidates = append(candidates, InboxPR{
			Number:     p.Number,
			Title:      p.Title,
			Author:     p.Author.Login,
			HeadBranch: p.HeadRefName,
			Updated:    updated,
			State:      ReviewPending,
		})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Updated.After(candidates[j].Updated) })
	return candidates, nil
}

// ClassifyReview reports what the user still owes on one PR (never reviewed,
// reviewed-then-updated, or reviewed-and-current).
func ClassifyReview(ctx context.Context, prNumber int) (ReviewState, error) {
	login, err := cachedLogin(ctx)
	if err != nil {
		return ReviewedCurrent, err
	}
	return reviewStateFor(ctx, prNumber, login)
}

var (
	loginOnce sync.Once
	loginVal  string
	loginErr  error
)

// cachedLogin returns the authenticated GitHub username, fetched once per process.
func cachedLogin(ctx context.Context) (string, error) {
	loginOnce.Do(func() {
		out, err := run(ctx, "api", "user", "-q", ".login")
		if err != nil {
			loginErr = err
			return
		}
		loginVal = strings.TrimSpace(out)
		if loginVal == "" {
			loginErr = fmt.Errorf("could not determine current gh user")
		}
	})
	return loginVal, loginErr
}

// reviewStateFor compares the user's latest review timestamp against the PR's
// latest commit to decide what (if anything) they still owe.
func reviewStateFor(ctx context.Context, prNumber int, login string) (ReviewState, error) {
	out, err := run(ctx, "pr", "view", strconv.Itoa(prNumber), "--json", "reviews,commits")
	if err != nil {
		return ReviewedCurrent, err
	}
	var data struct {
		Reviews []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			SubmittedAt string `json:"submittedAt"`
		} `json:"reviews"`
		Commits []struct {
			CommittedDate string `json:"committedDate"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return ReviewedCurrent, err
	}

	var myLatest time.Time
	for _, r := range data.Reviews {
		if !strings.EqualFold(r.Author.Login, login) {
			continue
		}
		if t, err := time.Parse(time.RFC3339, r.SubmittedAt); err == nil && t.After(myLatest) {
			myLatest = t
		}
	}
	if myLatest.IsZero() {
		return NotReviewed, nil
	}

	var lastCommit time.Time
	for _, c := range data.Commits {
		if t, err := time.Parse(time.RFC3339, c.CommittedDate); err == nil && t.After(lastCommit) {
			lastCommit = t
		}
	}
	if lastCommit.After(myLatest) {
		return UpdatedSinceReview, nil
	}
	return ReviewedCurrent, nil
}

func run(ctx context.Context, args ...string) (string, error) {
	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return string(out), nil
}
