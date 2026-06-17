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
wtree                             Dashboard picker (grouped worktrees + review inbox)
wtree add <input>                 Smart-add worktree from any input:
  PR URL                            https://github.com/owner/repo/pull/123
  Issue URL                         https://github.com/owner/repo/issues/45
  Project board URL                 https://github.com/orgs/Org/projects/1/...?issue=Org|repo|45
  Issue/PR number                   42 or #42
  Branch name                       feature/foo or pierce/my-branch
  New name                          my-feature (creates new branch)
wtree ls                          List all worktrees
wtree rm [name] [--force]         Remove worktree (auto-detects if inside one)
wtree sync [name] [--dry-run] [-y]  Re-copy configured ignored files from the primary
                                    repo into every child worktree (or just one)
wtree help                        Show help
```

`add` auto-detects the input type, queries GitHub when needed, shows a confirmation before creating anything, reuses an existing branch (local or remote) when the derived name already exists, and cd's into the resulting worktree on completion.

## Behavior

- Worktrees are created under `.worktrees/` at the repo root (auto-added to `.gitignore`)
- Running `wtree` with no args opens a **dashboard** of your current work. Worktrees are grouped by status — **Primary**, **In review** (open PR), **Merged · cleanup**, **In progress** (changes or commits ahead), **Idle** — and below them a **Needs my review** section lists open PRs that involve you (`is:open involves:@me -author:@me`) and that you still owe a review on, tagged `● not reviewed` or `↻ updated since your review`. PR numbers, associated issues, and the repo are clickable (OSC-8 links); `o` opens the selected row's PR/issue/repo in the browser and `i` opens its issue. Pressing `enter` on a review PR runs `wtree add` for it so you can review it locally. The `wtree rm` picker stays a flat list.
- Files are copied into new worktrees based on patterns in `~/.config/wtree/config.toml` (global) and `.wtree/config.toml` (per-repo); press `e` in the picker to open the repo config or `g` for the global one. No built-in defaults — nothing is copied if neither file exists.
- `wtree sync` re-runs that copy from the primary repo into existing worktrees so changes to your local-only config files propagate without recreating each worktree; it shows a per-worktree preview (new / overwrite / identical) and confirms before writing
- Post-create commands run in each new worktree via the `[commands]` config section (same global + per-repo files as the patterns). `post_create` is an ordered list; each entry is a bare string (a builtin recipe name or a shell command) or a table (`{ run = "...", if_exists = "<relpath>", required = false }`). Commands run with cwd at the worktree root, on `wtree add` only (not `sync`). `if_exists` skips a step unless the named path is present in the worktree — e.g. `{ run = "./admin reset", if_exists = "admin" }` to seed fixtures only where an admin runner exists.
- The `install-deps` builtin recipe installs dependencies in every directory with a lockfile (bun, npm, yarn, pnpm), highest-priority lockfile per dir, skipping `node_modules`/build/cache dirs — the recursive monorepo install. It's data-driven: override the lockfile table, skip-dirs, or command template under `[commands.install-deps]`, or omit it to use the built-in default. When a repo has **no** `[commands]` section at all, this install still runs automatically (back-compat); once `[commands]` exists, it's authoritative and you list `"install-deps"` explicitly. `WTREE_SKIP_INSTALL=1` skips it either way.
- Branches are prefixed with `pierce/` for repos not owned by McBrideMusings
- Issue-derived branch/worktree slugs are compacted by default to keep names shorter while preserving the issue number
- Optional tuning: `WTREE_ISSUE_WORD_LIMIT` (default `4`), `WTREE_ISSUE_SLUG_MAX_LEN` (default `36`), and `WTREE_SKIP_INSTALL=1` to skip the dependency install step

## Building from source

```bash
go build -o wtree .
```

## License

MIT
