# wtree

Go binary that wraps `git worktree`. The main logic lives in the binary; a small shell function in `~/.zshrc` lets the parent shell `cd` into the result.

## Structure

```
main.go                # entry point
cmd/                   # cobra subcommands (add, rm, ls, branches, picker dispatch)
internal/
  gitwt/               # git worktree wrappers (porcelain parsing, repo root, branch cleanup)
  classify/            # input classifier (PR URL / issue URL / number / text)
  slug/                # sanitize / issue-slug compaction (the only unit-tested piece)
  gh/                  # gh CLI wrapper for PR + issue lookups
  config/              # config loader (global ~/.config/wtree/config.toml + per-repo .wtree/config.toml)
  setup/               # post-create steps (symlink/copy configs per pattern lists, dep install)
  picker/              # Bubble Tea TUI worktree picker
  branchpicker/        # Bubble Tea TUI branch cleanup picker
  shim/                # CD sentinel emitter (__WTREE_CD__:<path>)
README.md
LICENSE
```

## Key Design Decisions

- **Binary + shell shim** — the binary handles all logic and prints `__WTREE_CD__:<absolute-path>` to stdout when it wants the parent shell to change directory. The shim function in `.zshrc` parses that sentinel and `cd`s. Same pattern as `zoxide`'s `z`. This is what allows `wtree rm` of the current worktree to leave the user in the main repo, without forcing the script to be sourced.
- **All non-CD output goes to stderr** — the sentinel is the only thing on stdout, so the shim's parser can't be confused by stray prints.
- **Worktrees go in `.worktrees/`** — created at the repo root of whatever project you're in, auto-added to `.gitignore`.
- **Branch prefix logic** — branches are prefixed with `pierce/` for repos not owned by McBrideMusings (see `gitwt.IsOwnRepo`). Repos without an origin remote are treated as own repos for now.
- **Smart `add` with auto-detection** — `wtree add <input>` classifies input as: GitHub PR URL, Issue URL, bare number (`#N` or `N`), or plain text (branch name). PR inputs check out the head branch; issue inputs construct a compacted `<num>-<slug>` from the title; numbers query GitHub to detect PR vs issue; plain text checks for existing branches before creating new ones. All paths confirm before creating.
- **Interactive picker** — `wtree` with no args opens a Bubble Tea picker. Keys: `enter` cd, `x` single remove, `D` batch-remove all merged worktrees (shows confirm screen listing them), `p` pull origin into the primary worktree (only shown when behind), `e` edit repo config, `g` edit global config, `q`/`esc` quit. The primary worktree is pinned as the first row (cd-only — `x`/remove on it flashes "Cannot remove the primary worktree"); the `wtree rm` picker still excludes it. Each row loads git status (uncommitted file count as `~N files`, `+N -N` line diff vs HEAD, commit age) and PR status (`✓ merged`, `#N` open, `✗ closed`) asynchronously on open — git status has a 3s timeout, PR status 10s via `gh pr list`. On open the picker also kicks off a background `git fetch origin <default>` (10s timeout) for the primary worktree; when local is behind, the main row gains a `↓N behind origin/<default>` marker and the `p` key fast-forward-pulls (`pull --ff-only`, refused when the primary is checked out on a non-default branch). Rows render as a table with consistent column widths (two-pass: measure plain-text widths, then render with padding). `D` on a repo where `gh` is unavailable shows a flash message rather than silently reporting no merged worktrees.
- **Branch cleanup** — `wtree branches` lists local branches eligible for deletion via two flows: (1) merged branches and non-personal branches (no commits by `git config user.email`) go to a batch confirm; (2) personal stale branches (> 4 days, at least one personal commit) go to an interactive picker with per-item toggles. Merged status checks `origin/<default>` first, falls back to local. Branches in any worktree are always excluded. See `internal/gitwt/branches.go` and `internal/branchpicker/`.
- **Symlink and copy patterns** — two config files control what gets set up in new worktrees: a global one at `~/.config/wtree/config.toml` and a per-repo `.wtree/config.toml`. Each has two pattern lists: `[symlink] patterns` creates live symlinks back to the primary worktree (directories get a single directory-level symlink), and `[copy] patterns` makes independent copies (walked recursively for directories). When both config files exist, their lists are merged (global first, local appended, duplicates removed). No built-in defaults — if neither config exists, nothing is set up. Use symlinks for files that should stay in sync across all worktrees; use copies for files that need to differ per branch (e.g. `.env` with different ports).
- **Re-sync to existing worktrees** — `wtree sync [name]` re-applies both pattern lists from the primary repo into every child worktree (or just the named one). Implementation: `setup.PlanAll(srcRoot, dstRoot)` loads config once and returns both `[]Change` (New/Overwrite/Identical, sha-256 content compare) and `[]SymlinkChange` (New/Exists/Wrong/Conflict). `setup.Apply` and `setup.ApplySymlinks` execute their respective plans. `setup.SetupConfigs` is the add-time entry point (calls `PlanAll` then both apply funcs). `cmd/sync.go` aggregates plans across all child worktrees, prints a per-target preview with `+`/`~`/`=` for copies and `L`/`~`/`=`/`!` for symlinks, and confirms before writing. Flags: `--dry-run`, `-y/--yes`. Source is always `gitwt.RepoRoot()`, so invoking from inside a child worktree still pulls from the primary.
- **Post-create commands** — a `[commands]` section (same global + per-repo config files) drives automation in new worktrees. `post_create` is an ordered `[]Step`; in TOML each entry is a bare string (a builtin recipe name, else a shell command) or a table (`run`/`if_exists`/`required`) — `Step.UnmarshalTOML` classifies the heterogeneous array. Steps run on `wtree add` only (not `sync`), after symlink/copy + direnv and before claude-plugins, with cwd at the worktree root via `sh -c`; `if_exists` gates a step on a worktree-relative path, `required` makes a failure abort the add (default: warn + continue). The recursive dependency installer is the data-driven builtin recipe `install-deps` (`internal/setup/install.go`): the lockfile→tool priority table, skip-dirs, and `{tool}`-templated command live in config with `DefaultInstallRecipe()` as the fallback, so nothing presupposes specific tools or directory names. `planInstalls` is the pure, unit-tested core. Back-compat: a repo with no `[commands]` section runs the legacy install automatically (`cmd/add.go`); once `[commands]` exists it's authoritative and `WriteDefault` seeds `post_create = ["install-deps"]`. `WTREE_SKIP_INSTALL=1` still skips installs.
- **Repo validation** — GitHub URLs are validated against the current repo's origin; mismatches show both repo names.
- **Tab completion** — cobra generates completion scripts; run `wtree completion zsh > _wtree` and place on your fpath if desired.

## Working on This Code

- `go build -o bin/wtree .` produces a binary at `bin/`. The repo's `.gitignore` excludes `bin/`.
- `go test ./...` — `internal/slug`, `internal/config` (parse/merge of patterns + the heterogeneous `post_create` array), and `internal/setup` (the `planInstalls` priority/prune logic and the `RunPostCreate` runner) have unit tests. The rest is thin glue over `git`/`gh` and is validated by manual end-to-end testing.
- Avoid hand-editing files in `internal/picker` without testing in a real terminal — Bubble Tea bugs typically only surface under a TTY.
- The `.zshrc` shim is documented in README.md. If you change the sentinel string, update both the binary (`internal/shim`) and the README simultaneously.
