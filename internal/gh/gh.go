// Package gh wraps the gh CLI for the small number of lookups wtree needs.
package gh

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

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
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
