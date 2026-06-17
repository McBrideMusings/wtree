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
	// NotReviewed means the user has submitted no review.
	NotReviewed ReviewState = iota
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

// reviewInboxQuery fetches, in one call, the open PRs involving the viewer
// (excluding their own) along with each PR's reviews and latest commit — enough
// to classify every PR locally without a per-PR follow-up call.
const reviewInboxQuery = `
query($q: String!) {
  viewer { login }
  search(query: $q, type: ISSUE, first: 30) {
    nodes {
      ... on PullRequest {
        number
        title
        updatedAt
        headRefName
        author { login }
        reviews(last: 50) { nodes { author { login } submittedAt } }
        commits(last: 1) { nodes { commit { committedDate } } }
      }
    }
  }
}`

// ReviewInbox returns open PRs in nwo ("owner/repo") that involve the user, were
// not authored by them, that they still owe a review on (never reviewed, or
// reviewed before the latest commit), and that aren't already checked out as a
// local worktree (those are shown as worktrees instead). A single GraphQL query
// fetches the search results plus each PR's reviews and latest commit, so the
// cost is one call regardless of PR count. Sorted most-recently-updated first.
func ReviewInbox(ctx context.Context, nwo string, localBranches []string) ([]InboxPR, error) {
	if nwo == "" {
		return nil, fmt.Errorf("no repo for review inbox")
	}
	out, err := run(ctx, "api", "graphql",
		"-f", "query="+reviewInboxQuery,
		"-f", "q=repo:"+nwo+" is:pr is:open involves:@me -author:@me")
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
			Search struct {
				Nodes []struct {
					Number      int    `json:"number"`
					Title       string `json:"title"`
					UpdatedAt   string `json:"updatedAt"`
					HeadRefName string `json:"headRefName"`
					Author      struct {
						Login string `json:"login"`
					} `json:"author"`
					Reviews struct {
						Nodes []struct {
							Author struct {
								Login string `json:"login"`
							} `json:"author"`
							SubmittedAt string `json:"submittedAt"`
						} `json:"nodes"`
					} `json:"reviews"`
					Commits struct {
						Nodes []struct {
							Commit struct {
								CommittedDate string `json:"committedDate"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
				} `json:"nodes"`
			} `json:"search"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, err
	}
	login := resp.Data.Viewer.Login

	local := make(map[string]bool, len(localBranches))
	for _, b := range localBranches {
		local[b] = true
	}

	var inbox []InboxPR
	for _, n := range resp.Data.Search.Nodes {
		if n.Number == 0 || local[n.HeadRefName] {
			continue // non-PR node, or already pulled down as a worktree
		}

		var myLatest time.Time
		for _, r := range n.Reviews.Nodes {
			if !strings.EqualFold(r.Author.Login, login) {
				continue
			}
			if t, err := time.Parse(time.RFC3339, r.SubmittedAt); err == nil && t.After(myLatest) {
				myLatest = t
			}
		}
		var lastCommit time.Time
		if len(n.Commits.Nodes) > 0 {
			lastCommit, _ = time.Parse(time.RFC3339, n.Commits.Nodes[0].Commit.CommittedDate)
		}

		var state ReviewState
		switch {
		case myLatest.IsZero():
			state = NotReviewed
		case lastCommit.After(myLatest):
			state = UpdatedSinceReview
		default:
			continue // reviewed and current — nothing owed
		}

		updated, _ := time.Parse(time.RFC3339, n.UpdatedAt)
		inbox = append(inbox, InboxPR{
			Number:     n.Number,
			Title:      n.Title,
			Author:     n.Author.Login,
			HeadBranch: n.HeadRefName,
			Updated:    updated,
			State:      state,
		})
	}
	sort.Slice(inbox, func(i, j int) bool { return inbox[i].Updated.After(inbox[j].Updated) })
	return inbox, nil
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
