---
description: Where the night is: its state and reason, one line per row, the next stop, and every `relay resume` line
allowed-tools: Bash(relay:*)
argument-hint: "[--night D]"
---

Run `relay status --json $ARGUMENTS` and print it. If the exit is 3 or `relay` is not found,
say `relay is not on PATH: make install` and stop.

From the JSON print, in plain lines and nothing else:

1. `night <night>: <state> (<reason>)`, without ` (<reason>)` when `reason` is empty (a night
   still `planning` has none); an empty `night` means no night has run: say so.
2. One line per `rows[]` entry: `<id>  <state>  <reason>  turns <turns>  context <context>/<ceiling>
   $<cost_usd>`, with `merge_sha` (short) when set, `last_tool` when the row is running, and
   its `worktree` indented under a red or failed row.
3. `burn`: sittings, turns, peak context, cost.
4. `Next: <next>` when `next` is not empty.
5. Every `resume[]` entry verbatim, one per line, under `Resume (in your own terminal):`.
   Each is `relay resume <row>` for a red or failed row. Never run them here.
