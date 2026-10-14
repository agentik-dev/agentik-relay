# agentik-relay

<img src="assets/icon.svg" alt="" width="56" align="left">

agentik-relay is a static Go binary, `agentik-relay` (`relay` a link to it until 2026-12-31), and a Claude Code plugin, `relay`, that run queued packet sittings headless overnight. Each queue row gets one git worktree, one branch and one sitting, and the row merges into `main` only when its gate reads green.

## Why it exists

- A household runs several Claude Code plugins, built in unattended relay runs: one sitting hands its work to the next.
- A person sleeps through the night and reads one report in the morning.
- So the night fails early (a pre-check, a circuit breaker, a wall clock, a spend cap), never pushes, never amends, never rebases, and leaves a door back into every stopped sitting.

## The door

- `agentik relay status`: the night's state and reason, one line per row, every resume line.
- `agentik relay resume <row>`: runs `claude --resume <session_id>` in the row's worktree, interactive. Every red or failed row carries this exact line in its ledger line, in `agentik relay status` and in `agentik relay report`.
- `/relay:resume`: the same door from inside Claude Code; prints the line and the worktree.
- Morning: `agentik relay report` (plain markdown: merged SHAs, stops verbatim, burn per sitting, branches waiting, resume lines, charter handovers).

## Install

```
claude plugin marketplace add agentik-dev/agentik-relay
claude plugin install relay@agentik
go install github.com/agentik-dev/agentik-relay/cmd/agentik-relay@latest
```

- From a clone: `make install`: `agentik-relay` into `BINDIR` (default `~/.local/bin`), `relay` a link to it until 2026-12-31, and the `agentik` front door beside it.
- Then `agentik relay doctor`: checks claude and its flags, git worktrees, caffeinate, tmux (a warning only), a writable state directory, and that the queue parses. Exit 0 or 2; an unknown result is never a pass.
- Requires: Go, git, the `claude` CLI. No plugin dependency is required.
- Optional: `respeak` (>=0.6.0) renders `/relay:report` as a narrative and renders this README; `agentik-session` is an optional override layer (its resolved config, read when present).
- tmux is optional (`tmux: auto`); without it each row gets a file window (a log file).

## The night

- `agentik relay plan --rows ar1,ar2 --until 06:00`: validate the queue and packets, resolve dependencies, snapshot origin refs, run the acceptance lines on `main` for a baseline, print each exact spawn line. Spawns nothing.
- `agentik relay run --until 06:00 --max-sittings 4 --max-usd 20 --parallel 2`: plan, then the loop. Ends on an empty queue, the count, the clock, an owner stop, or a failure.
- `agentik relay stop`: ask the night to stop after the running sittings; `agentik relay kill ar2`: stop a row now. Both write the night's control file, read after every event.
- `agentik relay attach ar2`: the row's tmux window; without tmux, `tail -f` of its log.
- `agentik relay gate ar2`: the gate by hand. `agentik relay clean --merged`: remove merged rows' worktrees; `agentik relay clean --stale`: remove dead relays' locks and their untouched worktrees.
- `agentik relay version`. Every verb takes `--json`.
- The queue is `relay/queue.yaml` in the target repo; packets are `design/packets/<id>.md`.
- State directory: `${XDG_STATE_HOME:-~/.local/state}/agentik-relay` (the old name's folder while only it exists) (ledger, `status.json`, logs, control file). The ledger never enters a repo.

## The plugin

Commands (each runs one verb with `--json`):

- `/relay:resume`: the door back into a stopped sitting.
- `/relay:status`: where the night is.
- `/relay:plan`: dry-run the queue.
- `/relay:gate`: gate one row by hand.
- `/relay:report`: the morning read; a narrative through respeak when installed.
- `/relay:doctor`: check the machine for a night.

Agents (for hand-run coordinator sittings):

- `builder`: builds one packet in its worktree; commits per act, never pushes.
- `adversary`: a read-only refuter over a branch; numbered findings with reproductions; fixes nothing.
- `attendant`: answers "where is the night" from the `--json` verbs and ledger lines.

Hooks are guards (belt and braces; nothing under the driver depends on them): PreToolUse denies amend, rebase and device verbs in every session, and push only in a sitting; Stop blocks a sitting's dirty tree or a sitting with no commit; SessionStart records a sitting's start and injects the packet for hand-run sittings. A sitting is a relay row (`RELAY_ROW` set) or a session started at a depth (`CLAUDE_SESSION_DEPTH` 1 or more); outside one, Stop never blocks and SessionStart records nothing, so the owner's own sessions stop freely.

## Configuration

Lowest first: shipped defaults; user file; repo file; session resolved file (when present); flags.

- User file `~/.config/agentik/relay/config.yaml`: `ntfy` (`url`, `topic`), `tmux` (`auto`), `allowed_tools`, `caffeinate` (`true`).
- Repo file `.claude/agentik/config.yaml`, section `relay` (else the old path and section, which doctor flags): `queue`, `merge`.

## Exit codes

| code | meaning |
|---|---|
| 0 | ok, green, or a night ended with no red |
| 1 | red, or a night stopped on a failure |
| 2 | usage, pre-check failed, or unwell (`doctor`) |
| 3 | not implemented |

## Design

- The design of record: `design/relay-r1.md` (the packet schema, the queue schema, the gate, the night's loop and its bounds).
- The packets, one per queue row: `design/packets/`.
- The sitting charters (goal, budget, pin, handover, exit line): `history/sittings/`.
- The TLA+ models of the night, with the two fork storms as violations TLC must find: `design/tla/`.

## Tests

- One targeted line: `go test ./internal/gate -run TestAR14_4_ -count=1`.
- `make check`: the gate under `scripts/bounded` (one per tree, an 840 s wall, held above load 4 x cores only when the CPU confirms the load): `go test -race ./...`, `go vet`, gofmt, doclint, hygiene, `claude plugin validate plugin/`, README freshness. No network, no claude call.
- `make e2e`: the night end to end over a fixture repo and a fake `claude`; never in check.
- `make tla`: TLC over `design/tla`; never fetches, never in check.

## Conventions

Shared with its sibling plugins:

- `scripts/hygiene` (the public-hygiene scan) and `scripts/doclint` (the doc gate) are canonical copies, pinned by sha256 in the Makefile. The scan holds only generic patterns (a home path, a LAN address, a secret's shape, an email address, the sibling plugins' names); the patterns that name anything private load from a list outside the repo, `$AGENTIK_HYGIENE_PRIVATE`, else `${XDG_CONFIG_HOME:-$HOME/.config}/agentik/hygiene-private.tsv`, one `group<TAB>regex` per line. Without the list the scan runs its generic groups and says so on stderr; `--require-private` makes a missing or unreadable list exit 2. It prints counts, never a matched line or a private pattern.
- `scripts/bounded`: a lock, a wall clock and a load ceiling around any long command. The load holds a check only when the CPU confirms it: an idle share at or under 25%.
- The README sidecar: edit `design/readme/source.md`, never `README.md`; `make readme` renders it and writes `design/readme/rendered.sha256` (source, README); `make check` fails a README whose sidecar does not match.

## Status

Status: version `0.1.3`, rendered `2026-10-13`, `699` Go tests.

Nothing is stable before `1.0.0`.

## Licence

MIT. See `LICENSE`.
