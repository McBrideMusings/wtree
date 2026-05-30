package setup

import (
	"os"
	"path/filepath"
	"testing"
)

// mkfile creates an empty file, making parent dirs as needed.
func mkfile(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// findPick returns the pick whose dir matches root/rel, or fails.
func pickFor(t *testing.T, picks []installPick, root, rel string) installPick {
	t.Helper()
	want := filepath.Join(root, rel)
	for _, p := range picks {
		if p.dir == want {
			return p
		}
	}
	t.Fatalf("no pick for %s in %+v", rel, picks)
	return installPick{}
}

func TestPlanInstallsPriorityAndPrune(t *testing.T) {
	root := t.TempDir()
	r := DefaultInstallRecipe()

	// root: both bun.lock and package-lock.json → bun wins (higher priority)
	mkfile(t, root, "bun.lock")
	mkfile(t, root, "package-lock.json")
	// subproject with only npm
	mkfile(t, root, "apps/api/package-lock.json")
	// subproject with pnpm
	mkfile(t, root, "apps/web/pnpm-lock.yaml")
	// a lockfile buried in a pruned dir must be ignored
	mkfile(t, root, "node_modules/dep/package-lock.json")

	picks := planInstalls(root, r)

	if len(picks) != 3 {
		t.Fatalf("want 3 picks (root, apps/api, apps/web), got %d: %+v", len(picks), picks)
	}
	if p := pickFor(t, picks, root, "."); p.cmd != "bun install" {
		t.Errorf("root cmd = %q, want bun install", p.cmd)
	}
	if p := pickFor(t, picks, root, "apps/api"); p.cmd != "npm install" {
		t.Errorf("apps/api cmd = %q, want npm install", p.cmd)
	}
	if p := pickFor(t, picks, root, "apps/web"); p.cmd != "pnpm install" {
		t.Errorf("apps/web cmd = %q, want pnpm install", p.cmd)
	}
}

func TestPlanInstallsCustomTemplateAndSkip(t *testing.T) {
	root := t.TempDir()
	r := DefaultInstallRecipe()
	r.Command = "{tool} ci --frozen"
	r.SkipDirs = []string{"vendor"} // narrow skip set; node_modules no longer pruned

	mkfile(t, root, "package-lock.json")
	mkfile(t, root, "vendor/pkg/bun.lock")                // pruned → ignored
	mkfile(t, root, "node_modules/dep/package-lock.json") // NOT pruned now → counted

	picks := planInstalls(root, r)
	if len(picks) != 2 {
		t.Fatalf("want 2 picks, got %d: %+v", len(picks), picks)
	}
	if p := pickFor(t, picks, root, "."); p.cmd != "npm ci --frozen" {
		t.Errorf("root cmd = %q, want template-substituted", p.cmd)
	}
}

func TestPlanInstallsEmptyTemplateFallsBack(t *testing.T) {
	root := t.TempDir()
	r := DefaultInstallRecipe()
	r.Command = ""
	mkfile(t, root, "yarn.lock")

	picks := planInstalls(root, r)
	if len(picks) != 1 || picks[0].cmd != "yarn install" {
		t.Fatalf("want yarn install fallback, got %+v", picks)
	}
}
