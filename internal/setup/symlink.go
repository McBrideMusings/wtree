package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/McBrideMusings/wtree/internal/config"
)

// SymlinkKind classifies what an ApplySymlinks call would do for one entry.
type SymlinkKind int

const (
	SymlinkNew      SymlinkKind = iota // nothing at dst — create symlink
	SymlinkExists                       // correct symlink already in place — no-op
	SymlinkWrong                        // symlink present but points elsewhere — re-link
	SymlinkConflict                     // regular file/dir at dst — skip
	SymlinkOrphaned                     // symlink points to srcRoot but pattern no longer active — remove
)

// SymlinkChange is one source→destination symlink in a PlanSymlinks result.
type SymlinkChange struct {
	Rel  string // path relative to the source/destination roots
	Src  string // absolute path in primary worktree
	Dst  string // absolute path in child worktree
	Kind SymlinkKind
}

// PlanSymlinks resolves the configured symlink patterns against srcRoot and
// reports the per-entry changes that linking into dstRoot would produce. It
// does not touch the filesystem under dstRoot. Each pattern match (file or
// directory) becomes a single symlink — directories are not walked.
func PlanSymlinks(srcRoot, dstRoot string) ([]SymlinkChange, error) {
	cfg, err := config.Load(srcRoot)
	if err != nil {
		return nil, err
	}
	return planSymlinks(cfg, srcRoot, dstRoot)
}

func planSymlinks(cfg *config.Config, srcRoot, dstRoot string) ([]SymlinkChange, error) {
	activeRels := make(map[string]bool)
	var changes []SymlinkChange
	for _, pattern := range cfg.Symlink.Patterns {
		matches, err := filepath.Glob(filepath.Join(srcRoot, pattern))
		if err != nil {
			return nil, fmt.Errorf("bad symlink pattern %q: %w", pattern, err)
		}
		for _, src := range matches {
			rel, err := filepath.Rel(srcRoot, src)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  (skip symlink %s: %v)\n", src, err)
				continue
			}
			activeRels[rel] = true
			dst := filepath.Join(dstRoot, rel)
			kind := classifySymlinkDst(src, dst)
			changes = append(changes, SymlinkChange{Rel: rel, Src: src, Dst: dst, Kind: kind})
		}
	}
	orphaned, err := findOrphaned(cfg, srcRoot, dstRoot, activeRels)
	if err != nil {
		return nil, err
	}
	return append(changes, orphaned...), nil
}

// findOrphaned scans only the parent directories implied by the active symlink
// patterns, looking for symlinks that target srcRoot but are no longer active.
func findOrphaned(cfg *config.Config, srcRoot, dstRoot string, activeRels map[string]bool) ([]SymlinkChange, error) {
	prefix := srcRoot + string(filepath.Separator)
	scanDirs := make(map[string]bool)
	for _, pattern := range cfg.Symlink.Patterns {
		scanDirs[filepath.Dir(pattern)] = true
	}
	var orphaned []SymlinkChange
	for relDir := range scanDirs {
		entries, err := os.ReadDir(filepath.Join(dstRoot, relDir))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			rel := filepath.Join(relDir, entry.Name())
			if activeRels[rel] {
				continue
			}
			dst := filepath.Join(dstRoot, rel)
			target, err := os.Readlink(dst)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(target, prefix) {
				continue
			}
			orphaned = append(orphaned, SymlinkChange{Rel: rel, Src: target, Dst: dst, Kind: SymlinkOrphaned})
		}
	}
	return orphaned, nil
}

func classifySymlinkDst(src, dst string) SymlinkKind {
	fi, err := os.Lstat(dst)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SymlinkNew
		}
		return SymlinkConflict
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return SymlinkConflict
	}
	target, err := os.Readlink(dst)
	if err != nil {
		return SymlinkConflict
	}
	if target == src {
		return SymlinkExists
	}
	return SymlinkWrong
}

// ApplySymlinks creates, repairs, or removes symlinks for the non-identical changes.
// Returns the number of symlink operations performed and the first error encountered.
func ApplySymlinks(changes []SymlinkChange) (int, error) {
	var firstErr error
	n := 0
	for _, c := range changes {
		switch c.Kind {
		case SymlinkExists:
			continue
		case SymlinkConflict:
			fmt.Fprintf(os.Stderr, "  (symlink conflict: %s is a real file, skipping)\n", c.Rel)
			continue
		case SymlinkOrphaned:
			if err := os.Remove(c.Dst); err != nil {
				fmt.Fprintf(os.Stderr, "  (failed to remove orphaned symlink %s: %v)\n", c.Rel, err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			n++
			continue
		case SymlinkWrong:
			if err := os.Remove(c.Dst); err != nil {
				fmt.Fprintf(os.Stderr, "  (failed to remove old symlink %s: %v)\n", c.Rel, err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			fallthrough
		case SymlinkNew:
			if err := os.MkdirAll(filepath.Dir(c.Dst), 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "  (failed to create dir for %s: %v)\n", c.Rel, err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if err := os.Symlink(c.Src, c.Dst); err != nil {
				fmt.Fprintf(os.Stderr, "  (failed to symlink %s: %v)\n", c.Rel, err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			n++
		}
	}
	return n, firstErr
}
