// Package setup runs the post-creation steps after a worktree is added:
// copying env/config files and installing dependencies.
package setup

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/McBrideMusings/wtree/internal/config"
)

// ChangeKind classifies what an Apply call would do for one file.
type ChangeKind int

const (
	ChangeNew       ChangeKind = iota // file does not exist at the destination
	ChangeOverwrite                   // file exists at the destination with different content
	ChangeIdentical                   // file exists with identical content (no-op on Apply)
)

// Change is one source→destination file copy in a Plan.
type Change struct {
	Rel  string // path relative to the source/destination roots
	Src  string
	Dst  string
	Kind ChangeKind
}

// Plan resolves the configured copy patterns against srcRoot and reports the
// per-file changes that writing into dstRoot would produce. It does not touch
// the filesystem under dstRoot. Directory matches are walked recursively.
func Plan(srcRoot, dstRoot string) ([]Change, error) {
	cfg, err := config.Load(srcRoot)
	if err != nil {
		return nil, err
	}
	return planCopy(cfg, srcRoot, dstRoot)
}

// PlanAll loads config once and returns both copy and symlink plans together.
func PlanAll(srcRoot, dstRoot string) ([]Change, []SymlinkChange, error) {
	cfg, err := config.Load(srcRoot)
	if err != nil {
		return nil, nil, err
	}
	copies, err := planCopy(cfg, srcRoot, dstRoot)
	if err != nil {
		return nil, nil, err
	}
	symlinks, err := planSymlinks(cfg, srcRoot, dstRoot)
	return copies, symlinks, err
}

func planCopy(cfg *config.Config, srcRoot, dstRoot string) ([]Change, error) {
	var changes []Change
	for _, pattern := range cfg.Copy.Patterns {
		matches, err := filepath.Glob(filepath.Join(srcRoot, pattern))
		if err != nil {
			return nil, fmt.Errorf("bad copy pattern %q: %w", pattern, err)
		}
		for _, src := range matches {
			info, err := os.Stat(src)
			if err != nil {
				continue
			}
			if info.IsDir() {
				walkDir(src, srcRoot, dstRoot, &changes)
				continue
			}
			if c, ok := makeChange(src, srcRoot, dstRoot); ok {
				changes = append(changes, c)
			}
		}
	}
	return changes, nil
}

func walkDir(srcDir, srcRoot, dstRoot string, out *[]Change) {
	filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "  (skip %s: %v)\n", path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if c, ok := makeChange(path, srcRoot, dstRoot); ok {
			*out = append(*out, c)
		}
		return nil
	})
}

func makeChange(src, srcRoot, dstRoot string) (Change, bool) {
	rel, err := filepath.Rel(srcRoot, src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  (skip %s: %v)\n", src, err)
		return Change{}, false
	}
	dst := filepath.Join(dstRoot, rel)
	kind := ChangeNew
	switch _, err := os.Stat(dst); {
	case err == nil:
		same, err := sameContent(src, dst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  (skip %s: %v)\n", rel, err)
			return Change{}, false
		}
		if same {
			kind = ChangeIdentical
		} else {
			kind = ChangeOverwrite
		}
	case errors.Is(err, os.ErrNotExist):
		// new file; kind stays ChangeNew
	default:
		fmt.Fprintf(os.Stderr, "  (skip %s: %v)\n", rel, err)
		return Change{}, false
	}
	return Change{Rel: rel, Src: src, Dst: dst, Kind: kind}, true
}

// Apply writes the non-identical changes to disk. Returns the number of files
// actually written and the first error encountered; per-file failures are
// logged to stderr and do not stop subsequent files.
func Apply(changes []Change) (int, error) {
	var firstErr error
	written := 0
	for _, c := range changes {
		if c.Kind == ChangeIdentical {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(c.Dst), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "  (failed to create dir for %s: %v)\n", c.Rel, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := copyFile(c.Src, c.Dst); err != nil {
			fmt.Fprintf(os.Stderr, "  (failed to copy %s: %v)\n", c.Rel, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		written++
	}
	return written, firstErr
}

// SetupConfigs runs both copy and symlink plans and applies them.
// This is the add-time entry point.
func SetupConfigs(srcRoot, dstRoot string) error {
	copies, symlinks, err := PlanAll(srcRoot, dstRoot)
	if err != nil {
		return err
	}
	if _, err := Apply(copies); err != nil {
		return err
	}
	_, err = ApplySymlinks(symlinks)
	return err
}

func sameContent(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	if ai.Size() != bi.Size() {
		return false, nil
	}
	ah, err := hashFile(a)
	if err != nil {
		return false, err
	}
	bh, err := hashFile(b)
	if err != nil {
		return false, err
	}
	return ah == bh, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return string(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
