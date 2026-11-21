# Sitting 2026-11-20-2: agentik-relay, rv1-binary-version

| item | value |
|---|---|
| goal | the binary says the plugin's version, the doctor's `binary` row compares them, and the `relay` section's `enabled` key is known (packet `design/packets/rv1-binary-version.md`); one commit of words for rr6's review |
| budget | about 90 tool calls, `claude-opus-5-5`; checkpoint at 80% |
| pin | `b281ef7` on main (plugin 0.2.5) |
| branch | `rv1-binary-version`, worktree `.claude/worktrees/rv1-binary-version`; nothing linked from main |
| level | S3 |
| window | 200k |
| agents | 0 |

## Done-ness
- [x] act 0, the packet
- [x] act 1, the tests (red, undefined names under `go vet`): `internal/effects/rv1_test.go`, `cmd/agentik-relay/rv1_test.go`; `TestVerbs` reads `relay unstamped`, `TestDoctorTable` gains `binary`, `TestAK8_2` counts eleven settings
- [ ] act 2, R1 to R4 (green)
- [ ] act 3, R5: the README clause, the plugin at 0.2.6
- [ ] act 4, rr6's review in words: its packet mended, NICE gains its rows
- [ ] `make check` through the gate queue

## Handover
- If this sitting is cut: the first unticked line above.

## Next act
Act 2, R1 to R4.

## Exit
(open)
