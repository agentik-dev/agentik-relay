---
description: Dry-run the queue: rows, their states, the problems, and the spawn line each row would run
allowed-tools: Bash(relay:*)
argument-hint: "[--queue PATH] [--rows a,b] [--until HH:MM]"
---

Run `relay plan --json $ARGUMENTS` and print it. If the exit is 3 or `relay` is not found,
say `relay is not on PATH: make install` and stop. Exit 2 means the plan is invalid: print it
anyway.

From the JSON print, in plain lines:

1. `night <night>, root <root>, valid <valid>`, and `until` and `stops` when set. `run_stop`
   names the stop the measure's budget makes: non-empty, `relay run` on this plan stops at
   once, so print it and raise the measure (`measure_seconds` in the queue, or
   `--measure-seconds`) before the run; `""` means the budget cut nothing, not that the run
   will start.
2. One block per `rows[]` entry: `<id>  <state>  <branch> off <base>`, its `worktree`, its
   `pending` dependencies when any, each of its `problems`, each of its `warnings` as
   `warning: <msg>` (never a problem: the row runs), and its `spawn` joined by spaces in a
   code block.
3. `verify` when set: `verify <name> on main: <status>, exit <main_exit> last "<main_last>"`
   (the merge takes the verify green or failing exactly so).
4. Every top-level `problems[]` entry as `<row>: <msg>`.
5. Nothing is started: `plan` never spawns. The night starts with `relay run` in a terminal.
