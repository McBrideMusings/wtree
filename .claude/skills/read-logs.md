---
name: read-logs
description: Read runtime logs from the last ./admin dev or other command. Use when the user says they ran the app and something didn't work, or when you need to check what happened during the last run.
---

# Read Logs

Admin commands write output to named log files in `tmp/`. The file name reflects the subcommand route:
- `./admin dev` → `tmp/dev.log`
- `./admin build` → `tmp/build.log`
- `./admin test` → `tmp/test.log`
- `./admin somecommand` → `tmp/somecommand.log`

Previous runs are retained as `.log.1`, `.log.2`, `.log.3` (most recent = `.1`).

## Strategy

Determine which command was last run, then read the corresponding file. Determine whether this is a **build problem** or a **runtime/logging problem**, then read accordingly.

### Build problem (binary didn't compile, crash on start)
Read from the **top** of the log file (first 80 lines). Look for:
- `error:` lines from the Go compiler
- `BUILD FAILED` or equivalent
- Crash output immediately after launch

### Runtime / behavior bug (binary ran but something went wrong)
Read from the **bottom** of the log file (last 80 lines). The user typically quits after observing the bug.

### If you need more context
- Read the full file only if the targeted read didn't give enough info
- Check the previous run at `.log.1` if the current log is empty or unrelated
- Search for specific error patterns

### What NOT to do
- Don't read the entire log file upfront if it's large
- Don't ask the user to paste logs — just read the file
- Don't run `./admin logs` to view logs — use the Read tool directly on the file
