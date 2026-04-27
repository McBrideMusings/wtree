package gitwt

import (
	"context"
	"strings"
)

// Worktree is one entry from `git worktree list --porcelain`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string // empty when Detached
	Detached bool
	Bare     bool
}

// IsMain reports whether this worktree is the primary worktree (the only one
// whose path lacks a parent linked-worktree gitdir).
func (w Worktree) IsMain(repoRoot string) bool {
	return w.Path == repoRoot
}

// List parses `git worktree list --porcelain` into structured entries.
func List(ctx context.Context) ([]Worktree, error) {
	out, err := runGitSilent(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var (
		result []Worktree
		cur    Worktree
		open   bool
	)
	flush := func() {
		if open {
			result = append(result, cur)
		}
		cur = Worktree{}
		open = false
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
			open = true
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			cur.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "detached":
			cur.Detached = true
		case line == "bare":
			cur.Bare = true
		}
	}
	flush()
	return result, nil
}

// FindByBranch returns the first non-main worktree whose branch matches.
func FindByBranch(list []Worktree, repoRoot, branch string) (Worktree, bool) {
	for _, w := range list {
		if w.Branch == branch && w.Path != "" && w.Path != repoRoot {
			return w, true
		}
	}
	return Worktree{}, false
}

// FindByPath returns the worktree at exactly path, if any.
func FindByPath(list []Worktree, path string) (Worktree, bool) {
	for _, w := range list {
		if w.Path == path {
			return w, true
		}
	}
	return Worktree{}, false
}
