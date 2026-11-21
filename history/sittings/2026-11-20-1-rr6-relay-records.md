# Sitting 2026-11-20-1: agentik-relay, rr6-relay-records

| item | value |
|---|---|
| goal | the relay refuses to plan, run, gate or resume in a repo whose records live outside its clone, and its doctor shows the state (packet `design/packets/rr6-relay-records.md`) |
| budget | about 90 tool calls, `claude-opus-5-5`; checkpoint at 80% |
| pin | `b735980` on main |
| branch | `rr6-relay-records`, worktree `.claude/worktrees/rr6-relay-records`; nothing linked from main |
| level | S3 |
| window | 200k |
| agents | 0 |

## Done-ness
- [x] act 0, the packet
- [x] act 1, the tests (red, undefined names under `go vet`): `internal/effects/records_test.go`, `cmd/agentik-relay/rr6_test.go`, `TestDoctorTable` gains the `records` row
- [x] act 2, R1 to R4: `effects.RecordsOut` and `Declares`; `recordsRefused` in plan, run, gate, resume before the pid file; doctor's `records` row after `root`; `go test ./internal/effects` ok; cmd's tests wait for the gate (`go test` there builds the relay)
- [x] act 3, R5: the README sentence beside the queue's (both files, the sidecar restamped, `shasum -c` OK); the plugin at 0.2.5 with TestAgentikR1_5's pin; no RUNBOOK.md in the repo
- [ ] `make check` through the gate queue: job `20261120-170317-2851` on `8b55bb1` red on two tests, `TestAR22_2_HandGateHoldsLock` (gate read the root before the lock) and `TestRR6_1` (each plan's fresh session id); both mended in `de33bc6`; job `20261120-171814-2859` on `c7eb007` exit 2: every code test green, TestAR20_5 and TestAR61_6 red on this packet's budget row (`about 90 tool calls`)

- [x] build 2, act 1: the packet's budget row in the parser's shape (`90 turns, 200k context`); `go test ./internal/packet` ok
- [ ] build 2, act 2: the coordinator's ruling: a set key `--path` cannot expand is refused (R1's added line, `TestRR6_3_KeyPathCannotExpand`)
- [ ] build 2: hygiene, then `make check` once through the gate queue

## Handover
- If this sitting is cut: the first unticked line above.
- decide, owner: the coordinator (the review): gate asks R1 under the account's lock, not before it, so ADV13-14's pin test stands; the refusal releases the lock and removes its pid file.
- ruled, the coordinator 2026-11-20: a set key that `--path` cannot expand (`~nouser/x`) is refused like any set key; build 2 act 2.

## Next act
Build 2 act 2, the unexpandable key refused; then hygiene and one check.

## Exit
Build 2 in flight.
