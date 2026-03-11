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
- **Uses `return 1` for errors** — since the script is sourced, `return` exits the script without killing the calling shell.
- **Smart `add` with auto-detection** — `wtree add <input>` classifies input as: GitHub PR URL, Issue URL, bare number (`#N` or `N`), or plain text (branch name). PR inputs check out the head branch; issue inputs construct `<num>-<sanitized-title>`; numbers query GitHub to detect PR vs issue; plain text checks for existing branches before creating new ones. All paths confirm before creating.
- **Interactive picker** — `wtree` with no args shows an arrow-key TUI to cd into or remove worktrees. The `rm` subcommand reuses the same picker when no target is specified.
- **Repo validation** — GitHub URLs are validated against the current repo's origin; mismatches show both repo names.
- **Tab completion** — a `_wtree` zsh completion function lives in `~/.zshrc` (not in this repo). It completes subcommands and worktree names for `rm`/`remove`.

## Working on This Script

- Test changes by opening a new shell (or `source ~/.zshrc`) and running `wtree` commands in any git repo.
- Tab completion changes require `source ~/.zshrc` or a new shell to take effect.
