# wtree

A Go-based git worktree helper that manages worktrees under `.worktrees/` with optional GitHub issue/PR integration.

## Install

Install the binary:

```bash
go install github.com/McBrideMusings/wtree@latest
```

Then add this function to your `.zshrc` (or `.bashrc`):

```zsh
wtree() {
  local out line cd_target=""
  out=$(command wtree "$@")
  local rc=$?
  while IFS= read -r line; do
    if [[ "$line" == "__WTREE_CD__:"* ]]; then
      cd_target="${line#__WTREE_CD__:}"
    else
      print -- "$line"
    fi
  done <<< "$out"
  [[ -n "$cd_target" ]] && cd -- "$cd_target"
  return $rc
}
```

The function captures stdout from the binary, watches for a `__WTREE_CD__:<path>` sentinel, and `cd`s the parent shell when one appears. This is what lets `wtree add` drop you into the new worktree and `wtree rm` cd back to the main repo when removing the current one.

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

`add` auto-detects the input type, queries GitHub when needed, shows a confirmation before creating anything, reuses an existing branch (local or remote) when the derived name already exists, and cd's into the resulting worktree on completion.

## Behavior

- Worktrees are created under `.worktrees/` at the repo root (auto-added to `.gitignore`)
- `.env*` files and `.claude/settings.local.json` are copied into new worktrees
- Dependencies are auto-installed in every directory with a lockfile (bun, npm, yarn, or pnpm), so subprojects in a monorepo get set up too — `node_modules`, `.git`, `.worktrees`, and common build/cache dirs are skipped during the scan
- Branches are prefixed with `pierce/` for repos not owned by McBrideMusings
- Issue-derived branch/worktree slugs are compacted by default to keep names shorter while preserving the issue number
- Optional tuning: `WTREE_ISSUE_WORD_LIMIT` (default `4`), `WTREE_ISSUE_SLUG_MAX_LEN` (default `36`), and `WTREE_SKIP_INSTALL=1` to skip the dependency install step

## Building from source

```bash
go build -o wtree .
```

## License

MIT
