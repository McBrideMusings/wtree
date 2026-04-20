# wtree

A bash wrapper around `git worktree` that manages worktrees under `.worktrees/` with optional GitHub issue integration.

## Install

Add to your `.zshrc`:

```bash
wtree() { source ~/Projects/wtree/wtree "$@"; }
```

The script must be `source`d (not executed) so that `wtree rm` can `cd` the calling shell back to the main repo when removing the current worktree.

## Usage

```
wtree                             Interactive picker (cd/remove worktrees)
wtree add <input>                 Smart-add worktree from any input:
  PR URL                            https://github.com/owner/repo/pull/123
  Issue URL                         https://github.com/owner/repo/issues/45
  Issue/PR number                   42 or #42
  Branch name                       feature/foo or pierce/my-branch
  New name                          my-feature (creates new branch)
wtree ls                          List all worktrees
wtree rm [name] [--force]         Remove worktree (auto-detects if inside one)
wtree help                        Show help
```

`add` auto-detects the input type, queries GitHub when needed, shows a confirmation before creating anything, and offers to cd into an existing worktree if one already matches.

## Behavior

- Worktrees are created under `.worktrees/` at the repo root (auto-added to `.gitignore`)
- `.env*` files and `.claude/settings.local.json` are copied into new worktrees
- Dependencies are auto-installed (bun, npm, yarn, or pnpm based on lockfile)
- Branches are prefixed with `pierce/` for repos not owned by McBrideMusings
- Issue-derived branch/worktree slugs are compacted by default to keep names shorter while preserving the issue number
- Optional tuning: `WTREE_ISSUE_WORD_LIMIT` (default `4`) and `WTREE_ISSUE_SLUG_MAX_LEN` (default `36`)

## License

MIT
