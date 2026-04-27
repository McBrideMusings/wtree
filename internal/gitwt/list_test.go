package gitwt

import (
	"context"
	"testing"
)

// Live test: requires the test repo to have at least one detached worktree
// at `.worktrees/detached-test`. Skips otherwise.
func TestListDetached(t *testing.T) {
	list, err := List(context.Background())
	if err != nil {
		t.Skipf("git worktree list failed (not in a repo?): %v", err)
	}
	for _, w := range list {
		if w.Detached {
			if w.Branch != "" {
				t.Errorf("detached worktree %q should have empty Branch, got %q", w.Path, w.Branch)
			}
			if w.Head == "" {
				t.Errorf("detached worktree %q should have non-empty Head", w.Path)
			}
			return
		}
	}
	t.Skip("no detached worktree present; nothing to assert")
}
