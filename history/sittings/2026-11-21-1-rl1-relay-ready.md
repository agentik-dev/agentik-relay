# Sitting 2026-11-21-1: agentik-relay, rl1-relay-ready

| item | value |
|---|---|
| goal | the gate green with the clone's records gone: six tests read fixtures (R1, R2), the whole check rehearsed in a scratch clone with the records removed (R3), nothing weakened (R4) |
| budget | about 90 tool calls, `claude-opus-5-5`; checkpoint at 80% |
| pin | `a2e6ec7` on main |
| branch | `rl1-relay-ready`, worktree `.claude/worktrees/rl1-relay-ready`; nothing linked from main |
| level | S3 |
| window | 200k |
| agents | 0 |

## Done-ness
- [x] act 0: the packet
- [x] act 1: R1, `ar10_test.go` and `ar21_4_test.go` read their packet's copy under `internal/packet/testdata/packets/`, the bytes at the pin; packet and queue `ok`
- [x] act 2: R2, the four globbing tests read a fixed sample: `testdata/packets/` the 13 spent, ar21 (bound) and ar8 (legacy); `testdata/peract/` ar51 and ar24; `internal/queue/testdata/rows/` one rows file; each file's mutation red, then green
- [x] act 3: R3, the rehearsal's `make check` green through the gate: job `20261121-101533-3051` exit 0 in 6 min on the scratch clone's head over `2de57c2`; nothing else red
- [ ] the gate's `make check` on this head (queued after this commit); hygiene over `main..HEAD`: 21 sibling hits, all in `be35ebb`, the tree scan 0

## Handover
- If this sitting is cut: the first unticked line above.
- decide, owner: the coordinator (the review): R1's two copies hold the pin's bytes but for the words the live-file scan refuses (old tool names, sibling words), each swapped for a made-up word of the same length, as R2 rules for every fixture; their tests' verdicts unchanged.
- decide, owner: the coordinator (the landing): commit `be35ebb` added the samples with the sibling group's words, which `--commits main..HEAD` still counts after `2de57c2` swapped them; no allow row added here. The rows it needs: `internal/packet/testdata/packets/*.md: sibling` and `internal/packet/testdata/peract/*.md: sibling`; the lines hold only the public sibling plugins' names, as the `design/packets/*.md` row already allows.

## Next act
The coordinator's read; the allow rows the second decide line names, before the landing.

## Exit
R1 to R3 built and committed, the rehearsal green (job `20261121-101533-3051`); this head's own check is queued after this commit and its id is in the hand-back; `--commits main..HEAD` waits on the coordinator's allow rows.
