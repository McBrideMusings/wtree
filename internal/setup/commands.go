package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/McBrideMusings/wtree/internal/config"
)

// RunPostCreate executes the configured post_create steps in order inside the
// new worktree. Each step is either a builtin recipe (e.g. "install-deps") or a
// shell command run via `sh -c` with cwd set to the worktree root. A step's
// IfExists gate skips it unless the named path exists in the worktree. A failing
// step is reported and skipped unless it is marked Required, in which case the
// error is returned to abort the add.
func RunPostCreate(repoRoot, worktreePath string, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	for _, step := range cfg.Commands.PostCreate {
		if step.IfExists != "" {
			if _, err := os.Stat(filepath.Join(worktreePath, step.IfExists)); err != nil {
				fmt.Fprintf(os.Stderr, "  (skip step: %s not present)\n", step.IfExists)
				continue
			}
		}

		if step.Recipe != "" {
			if err := runRecipe(step.Recipe, worktreePath, cfg); err != nil {
				if step.Required {
					return err
				}
				fmt.Fprintf(os.Stderr, "  (%s warning: %v)\n", step.Recipe, err)
			}
			continue
		}

		fmt.Fprintf(os.Stderr, "Running: %s\n", step.Run)
		cmd := exec.Command("sh", "-c", step.Run)
		cmd.Dir = worktreePath
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if step.Required {
				return fmt.Errorf("post_create command failed (%s): %w", step.Run, err)
			}
			fmt.Fprintf(os.Stderr, "  (command failed: %s)\n", step.Run)
		}
	}
	return nil
}

// runRecipe dispatches a builtin recipe by name.
func runRecipe(name, worktreePath string, cfg *config.Config) error {
	switch name {
	case "install-deps":
		recipe := DefaultInstallRecipe()
		if cfg.Commands.InstallDeps != nil {
			recipe = *cfg.Commands.InstallDeps
		}
		return InstallDepsWithRecipe(worktreePath, recipe)
	default:
		return fmt.Errorf("unknown recipe %q", name)
	}
}
