---
description: Check the machine for a night: claude, git, tmux, ntfy, the state dir, the queue, the config
allowed-tools: Bash(relay:*)
---

Run `relay doctor --json`. If the exit is 3 or `relay` is not found, say
`relay is not on PATH: make install` and stop (the hooks stay quiet without it).

From the JSON print one line per `checks[]` entry: `ok`, `warn` or `FAIL`, then `name`, then
`detail`. `FAIL` when `ok` is false; `warn` when `ok` and `warn` are true; else `ok`. End with
`doctor: ok` when the top-level `ok` is true, else `doctor: <n> failing` and, for each failing
check, its detail once more as the thing to fix.
