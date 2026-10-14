---
description: Run the gate on one row by hand: the result, the facts, and each acceptance line on main and on the branch
allowed-tools: Bash(agentik-relay *)
argument-hint: "<row>"
---

Run `agentik-relay gate --json $ARGUMENTS`; it needs one row. If the exit is 3 or `relay` is not
found, say `agentik-relay is not on PATH: make install` and stop. Exit 0 is green, 1 red or failed,
2 unrun (print `error`).

From the JSON print, in plain lines:

1. `<row>: <result> (<reason>)`, and `error` when set.
2. The facts: `exit_ok`, `result_seen`, `commit`, `clean`, `origin_same`, and each `outside`
   path.
3. One line per `lines[]` entry: `<status>  <name>`, then indented `want <expect>; main exit
   <main_exit> last "<main_last>"; branch exit <branch_exit> last "<branch_last>"` (a main exit
   of -1 means main was not measured).
4. When the result is red or failed, the resume line `agentik relay resume <row>` verbatim, to run in
   your own terminal, never here.
