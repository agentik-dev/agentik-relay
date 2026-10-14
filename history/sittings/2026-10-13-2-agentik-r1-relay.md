# Sitting 2026-10-13-2: agentik-relay, agentik-r1-relay (the relay takes its agentik names)

| item | value |
|---|---|
| goal | packet `design/packets/agentik-r1-relay.md`: guards on every spelling, the vendored commons files, paths new else old, one resume line, `cmd/agentik-relay` and plugin `relay`, `renamed` 0 outside the allowlist |
| budget | claude-opus-5-5 builder (the caller's): 160 turns, 170k context, about 90 tool calls; one commit per act |
| pin | agentik-relay main `5224f8c`; agentik-commons main `a609b0f` (public); worktree `~/.cache/agentik-worktrees/agentik-r1-relay`, branch `agentik-r1-relay` |
| window | 1M, held to the caller's 170k |
| agents | none; no relay binary built or run; package tests per act; `make check` once, last, under load 12 |
| wall | this worktree only; no push, merge, rebase, amend; no relay verb; no install; no `~/.local/state` |

## Done-ness

| act | what | state |
|---|---|---|
| 1 | guards on every spelling (c12); the session guard `nested-session`; `charter` exempt | done `dccd8a9` |
| 2 | vendor `scripts/{agentik,hygiene,bounded}` + `vendored.sha256`; `make hygiene` checks it | done `e62cd14` |
| 3 | paths new, else old (c1-c4, c6); `Config.Warnings` | done `5f3ebb5` |
| 4 | one resume line (`relay.ResumeLine`, `ResumeRow`); the alias note | done `1bd305c` |
| 5 | `cmd/agentik-relay`, the Makefile, the plugin (c11, c14, D2, D3) | done |
| 6 | the sweep to `renamed` 0 | |

Rulings beyond the packet, one line each:

- The caller's word: charter allow rows (ruling 10's `history/sittings` part) are dropped; no
  `.hygiene-live` is declared, so no charter is live.
- Charter finding: a row WAS denied `claude-session charter open` (red on the pin);
  `nested-session` exempts `charter` under all three spellings by its `Unless`.
- `relayName` reads `agentik relay` as the relay only when `relay` is the next word, as
  `scripts/agentik` takes its tool as its first argument.
- Act 2: `internal/hygiene`'s hit exits read 1 (four checks), its bad-line exit 2 (one);
  `--require-private` stays 2.
- Act 3: c4 warns whenever the repo's old file exists (read or not); an old `agent-relay` section
  alone is read without a warning (c6 warns only with both); the pid door checks PidDir's path
  and both spellings beside it; tests that used an old path as the default take the new one.

## Handover

(none yet)

## Next act

Act 6: the sweep to `renamed` 0.

## Exit

(open)
