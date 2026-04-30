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
  config/              # .wtree/config.toml loader (copy patterns, falls back to defaults)
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
- **Interactive picker** — `wtree` with no args opens a Bubble Tea picker to cd into or remove worktrees. `wtree rm` reuses the same picker when invoked from the main repo with no target. The picker filters out the main worktree. Press `e` to open `.wtree/config.toml` in `$EDITOR`/`$VISUAL` (creates the file with defaults if absent).
- **Configurable copy patterns** — `.wtree/config.toml` at the repo root controls which files are copied into new worktrees. Defaults: `.env*`, `.dev.vars`, `.claude/settings.local.json`. Absence of the file is equivalent to using the defaults; an empty `patterns = []` disables all copying.
- **Repo validation** — GitHub URLs are validated against the current repo's origin; mismatches show both repo names.
- **Tab completion** — cobra generates completion scripts; run `wtree completion zsh > _wtree` and place on your fpath if desired.

## Working on This Code

- `go build -o bin/wtree .` produces a binary at `bin/`. The repo's `.gitignore` excludes `bin/`.
- `go test ./...` — only `internal/slug` has unit tests (the most regression-prone piece). Everything else is thin glue over `git`/`gh` and is validated by manual end-to-end testing.
- Avoid hand-editing files in `internal/picker` without testing in a real terminal — Bubble Tea bugs typically only surface under a TTY.
- The `.zshrc` shim is documented in README.md. If you change the sentinel string, update both the binary (`internal/shim`) and the README simultaneously.
