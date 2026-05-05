// Package gh wraps the gh CLI for the small number of lookups wtree needs.
package gh

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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
