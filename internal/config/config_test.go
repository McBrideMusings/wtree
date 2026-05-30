package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

// decode is a small helper to parse a TOML body into a Config.
func decode(t *testing.T, body string) Config {
	t.Helper()
	var c Config
	if _, err := toml.Decode(body, &c); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return c
}

func TestPostCreateHeterogeneousArray(t *testing.T) {
	c := decode(t, `
[commands]
post_create = [
  "install-deps",
  "bun run setup",
  { run = "./admin reset", if_exists = "admin", required = true },
]
`)
	steps := c.Commands.PostCreate
	if len(steps) != 3 {
		t.Fatalf("want 3 steps, got %d", len(steps))
	}
	// bare string matching a known recipe → recipe
	if steps[0].Recipe != "install-deps" || steps[0].Run != "" {
		t.Errorf("step0 = %+v, want recipe install-deps", steps[0])
	}
	// bare string that is not a recipe → shell command
	if steps[1].Run != "bun run setup" || steps[1].Recipe != "" {
		t.Errorf("step1 = %+v, want run command", steps[1])
	}
	// table form with options
	if steps[2].Run != "./admin reset" || steps[2].IfExists != "admin" || !steps[2].Required {
		t.Errorf("step2 = %+v, want run+if_exists+required", steps[2])
	}
}

func TestInstallDepsOverrideParses(t *testing.T) {
	c := decode(t, `
[commands]
post_create = ["install-deps"]

[commands.install-deps]
command = "pnpm i"
skip_dirs = ["node_modules", "vendor"]
[[commands.install-deps.lockfiles]]
file = "pnpm-lock.yaml"
tool = "pnpm"
`)
	r := c.Commands.InstallDeps
	if r == nil {
		t.Fatal("install-deps override is nil")
	}
	if r.Command != "pnpm i" {
		t.Errorf("command = %q", r.Command)
	}
	if len(r.SkipDirs) != 2 || r.SkipDirs[1] != "vendor" {
		t.Errorf("skip_dirs = %v", r.SkipDirs)
	}
	if len(r.Lockfiles) != 1 || r.Lockfiles[0].Tool != "pnpm" {
		t.Errorf("lockfiles = %+v", r.Lockfiles)
	}
}

func TestMergeCommandsDedupAndOverride(t *testing.T) {
	global := decode(t, `
[commands]
post_create = ["install-deps"]
[commands.install-deps]
command = "global"
`)
	local := decode(t, `
[commands]
post_create = ["install-deps", { run = "./admin reset", if_exists = "admin" }]
[commands.install-deps]
command = "local"
`)
	merged := mergeCommands(global.Commands, local.Commands)

	// install-deps appears once despite being in both lists
	recipeCount := 0
	for _, s := range merged.PostCreate {
		if s.Recipe == "install-deps" {
			recipeCount++
		}
	}
	if recipeCount != 1 {
		t.Errorf("install-deps appears %d times, want 1", recipeCount)
	}
	if len(merged.PostCreate) != 2 {
		t.Errorf("want 2 merged steps, got %d: %+v", len(merged.PostCreate), merged.PostCreate)
	}
	// local override wins
	if merged.InstallDeps == nil || merged.InstallDeps.Command != "local" {
		t.Errorf("install-deps override = %+v, want local", merged.InstallDeps)
	}
}

func TestMergePatternsStillDedup(t *testing.T) {
	got := mergePatterns([]string{"a", "b"}, []string{"b", "c"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
