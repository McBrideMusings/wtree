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
wtree add <name>                  Create worktree in .worktrees/<name>
wtree add --branch <branch>       Check out existing branch into worktree
wtree add --issue <num>           Fetch GitHub issue, create <num>-<title>
wtree add --issue "title"         Create new GitHub issue, create <num>-<title>
wtree ls                          List all worktrees
wtree rm [name] [--force]         Remove worktree (auto-detects if inside one)
```

## Behavior

- Worktrees are created under `.worktrees/` at the repo root (auto-added to `.gitignore`)
- `.env*` files and `.claude/settings.local.json` are copied into new worktrees
- Dependencies are auto-installed (bun, npm, yarn, or pnpm based on lockfile)
- Branches are prefixed with `pierce/` for repos not owned by McBrideMusings

## License

MIT
