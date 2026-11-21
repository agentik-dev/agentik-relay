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
- [ ] act 3, R5: the README sentence (both files, the sidecar) and the plugin at 0.2.5
- [ ] `make check` through the gate queue

## Handover
- If this sitting is cut: the first unticked line above.

## Next act
Act 3, R5: the README sentence and the plugin at 0.2.5.

## Exit
