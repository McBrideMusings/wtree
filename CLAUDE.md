# wtree

Go binary that wraps `git worktree`. The main logic lives in the binary; a small shell function in `~/.zshrc` lets the parent shell `cd` into the result.

## Structure

```
main.go                # entry point
cmd/                   # cobra subcommands (add, rm, ls, picker dispatch)
internal/
  gitwt/               # git worktree wrappers (porcelain parsing, repo root)
  classify/            # input classifier (PR URL / issue URL / number / text)
  slug/                # sanitize / issue-slug compaction (the only unit-tested piece)
  gh/                  # gh CLI wrapper for PR + issue lookups
  config/              # config loader (global ~/.config/wtree/config.toml + per-repo .wtree/config.toml)
  setup/               # post-create steps (copy configs per pattern list, dep install)
  picker/              # Bubble Tea TUI picker
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
- **Interactive picker** — `wtree` with no args opens a Bubble Tea picker. Keys: `enter` cd, `x` single remove, `D` batch-remove all merged worktrees (shows confirm screen listing them), `e` edit repo config, `g` edit global config, `q`/`esc` quit. Each row loads git status (dirty flag, commit age) and PR status (`✓ merged`, `#N` open, `✗ closed`) asynchronously on open — git status has a 1s timeout, PR status 10s via `gh pr list`. `D` on a repo where `gh` is unavailable shows a flash message rather than silently reporting no merged worktrees.
- **Configurable copy patterns** — two config files control which files are copied into new worktrees: a global one at `~/.config/wtree/config.toml` (personal baseline across all repos) and a per-repo `.wtree/config.toml`. When both exist, their pattern lists are merged (global first, local appended, duplicates removed). When only one exists, it is used as-is; an empty `patterns = []` in the only present file disables all copying. Built-in defaults (`.env*`, `.dev.vars`, `.claude/settings.local.json`, `admin`, `admin.toml`) apply only when neither file exists.
- **Repo validation** — GitHub URLs are validated against the current repo's origin; mismatches show both repo names.
- **Tab completion** — cobra generates completion scripts; run `wtree completion zsh > _wtree` and place on your fpath if desired.

## Working on This Code

- `go build -o bin/wtree .` produces a binary at `bin/`. The repo's `.gitignore` excludes `bin/`.
- `go test ./...` — only `internal/slug` has unit tests (the most regression-prone piece). Everything else is thin glue over `git`/`gh` and is validated by manual end-to-end testing.
- Avoid hand-editing files in `internal/picker` without testing in a real terminal — Bubble Tea bugs typically only surface under a TTY.
- The `.zshrc` shim is documented in README.md. If you change the sentinel string, update both the binary (`internal/shim`) and the README simultaneously.
