# Sitting 2026-10-13-2: agentik-r1-relay (builder)

- **Goal:** packet `design/packets/agentik-r1-relay.md`: the relay takes its agentik names
  (guards on every spelling, vendored commons files, new-else-old paths, one resume line,
  `cmd/agentik-relay`, plugin `relay`), and `renamed` reads 0 outside the allowlist.
- **Budget:** 160 turns, 170k context (caller); about 90 tool calls (builder bound).
- **Pin:** agentik-relay main 5224f8c; agentik-commons main a609b0f (public).
- **Worktree:** `~/.cache/agentik-worktrees/agentik-r1-relay`, branch `agentik-r1-relay`.

## Acts

| act | what | state |
|---|---|---|
| 1 | guards on every spelling (c12); the session guard `nested-session`; `charter` exempt | done |
| 2 | vendor `scripts/{agentik,hygiene,bounded}` + `vendored.sha256`; `make hygiene` checks it | |
| 3 | paths new, else old (c1-c4, c6); `Config.Warnings` | |
| 4 | one resume line (`relay.ResumeLine`, `ResumeRow`); the alias note | |
| 5 | `cmd/agentik-relay`, the Makefile, the plugin (c11, c14, D2, D3) | |
| 6 | the sweep to `renamed` 0 | |

## Rulings beyond the packet

- The caller's word: charter allow rows (ruling 10's `history/sittings` part) are dropped;
  no `.hygiene-live` is declared, so no charter is live.
- Charter finding: a row WAS denied `claude-session charter open` (red on the pin); `nested-session`
  exempts `charter` under all three spellings by its `Unless`; every other verb stays denied.
- `relayName` reads `agentik relay` as the relay only when `relay` is the very next word, as
  `scripts/agentik` takes its tool as its first argument.

## Handover

(none yet)

## Next act

Act 2.

## Exit

(open)
