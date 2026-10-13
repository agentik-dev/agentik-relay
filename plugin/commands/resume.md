---
description: The door back into a stopped sitting: print the exact `relay resume <row>` line and worktree for your own terminal
allowed-tools: Bash(relay:*)
argument-hint: "[<row>] [--night D]"
---

Show how to resume a sitting. Never run `relay resume <row>` or `claude --resume` here: both
start a nested interactive session. Only read the status.

1. Run `relay status --json` (add `--night D` when `$ARGUMENTS` carries one). If the exit is 3
   or `relay` is not found, say `relay is not on PATH: make install` and stop.
2. From the JSON: `night`, and `rows[]` with `id`, `worktree`, `state`, `reason` and `resume`.
3. When `$ARGUMENTS` names a row: print its `id`, `state`, `reason` and `worktree`, then its
   `resume` line verbatim in a code block. When the row has no `resume` line, say it is not red or
   failed and needs no resume. An unknown row is an error: list the known ids.
4. When no row is named: one block per row with a non-empty `resume`, the same four facts
   and the line verbatim. When there are none, print `nothing to resume on night <night>`.
5. After any resume line, print `relay resume <row>` prints `cd <worktree> && claude --resume
   <session>`, with the row's `worktree` from the status filled in.
6. End with one line: `run it in your own terminal, not here`.
