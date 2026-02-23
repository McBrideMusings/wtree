# wtree

Bash wrapper around `git worktree`. Single-file script, no build system.

## Structure

```
wtree          # The script (sourced, not executed)
README.md
LICENSE        # MIT
.gitignore
```

## Key Design Decisions

- **Must be `source`d** — `wtree rm` needs to `cd` the calling shell back to the main repo when removing the current worktree. This requires a shell wrapper function in `.zshrc`:
  ```bash
  wtree() { source ~/Projects/wtree/wtree "$@"; }
  ```
- **No install script** — sourcing directly from the repo means edits take effect immediately in new shells, and updating is just `git pull`.
- **Worktrees go in `.worktrees/`** — created at the repo root of whatever project you're in, auto-added to `.gitignore`.
- **Branch prefix logic** — branches are prefixed with `pierce/` for repos not owned by McBrideMusings (see `_wtree_is_own_repo`).

## Working on This Script

- Test changes by opening a new shell (or `source ~/.zshrc`) and running `wtree` commands in any git repo.
- The script uses `exit 1` for errors (not `return`) since it's sourced inside a function wrapper — `exit` terminates the sourced script, not the shell.
