package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/McBrideMusings/wtree/internal/config"
)

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestRunPostCreateRunsAndGates(t *testing.T) {
	wt := t.TempDir()
	cfg := &config.Config{Commands: config.CommandsConfig{
		PostCreate: []config.Step{
			{Run: "touch ran.txt"},                         // runs in worktree cwd
			{Run: "touch skipped.txt", IfExists: "absent"}, // gated out
		},
	}}
	if err := RunPostCreate(wt, wt, cfg); err != nil {
		t.Fatalf("RunPostCreate: %v", err)
	}
	if !exists(filepath.Join(wt, "ran.txt")) {
		t.Error("ran.txt was not created — command did not run in worktree cwd")
	}
	if exists(filepath.Join(wt, "skipped.txt")) {
		t.Error("skipped.txt exists — if_exists gate did not skip the step")
	}
}

func TestRunPostCreateRequiredFailureAborts(t *testing.T) {
	wt := t.TempDir()
	cfg := &config.Config{Commands: config.CommandsConfig{
		PostCreate: []config.Step{{Run: "false", Required: true}},
	}}
	if err := RunPostCreate(wt, wt, cfg); err == nil {
		t.Error("expected error from required failing step, got nil")
	}
}

func TestRunPostCreateInstallRecipeDispatch(t *testing.T) {
	wt := t.TempDir()
	mkfile(t, wt, "lock.test")
	// override the recipe so "install" is a harmless `true` (no network).
	cfg := &config.Config{Commands: config.CommandsConfig{
		PostCreate: []config.Step{{Recipe: "install-deps"}},
		InstallDeps: &config.InstallRecipe{
			Command:   "true {tool}",
			Lockfiles: []config.LockfileRule{{File: "lock.test", Tool: "x"}},
		},
	}}
	if err := RunPostCreate(wt, wt, cfg); err != nil {
		t.Fatalf("RunPostCreate with install recipe: %v", err)
	}
}

func TestRunPostCreateUnknownRecipeWarnsNotFatal(t *testing.T) {
	wt := t.TempDir()
	cfg := &config.Config{Commands: config.CommandsConfig{
		PostCreate: []config.Step{{Recipe: "bogus"}},
	}}
	if err := RunPostCreate(wt, wt, cfg); err != nil {
		t.Errorf("unknown non-required recipe should warn, not error: %v", err)
	}
}
