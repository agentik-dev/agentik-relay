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
- [ ] act 1: R1, `ar10_test.go` and `ar21_4_test.go` read their packet's copy
- [ ] act 2: R2, the four globbing tests read a fixed sample
- [ ] act 3: R3, the rehearsal's `make check` green through the gate; its reds mended
- [ ] the gate's `make check` on the head handed back; hygiene over `main..HEAD` 0 hits

## Handover
- If this sitting is cut: the first unticked line above.

## Next act
Act 1: R1.

## Exit
Open.
