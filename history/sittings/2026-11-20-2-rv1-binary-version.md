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
- [x] act 2, R1 to R4: `var version = "unstamped"`, the Makefile's `STAMP` on build and install, `binary-version-check` in `check-unlocked`; the doctor's `binary` row before `template`; `enabled` known with five choices, `describe config`'s eleventh setting; `go test ./internal/effects` ok; cmd's tests wait for the gate
- [x] act 3, R5: the README's version clause in both files, the sidecar restamped (`shasum -c` OK); the plugin at 0.2.6 with TestAgentikR1_5's pin; `go test ./internal/packet` ok
- [x] act 4, rr6's review in words: its packet's R1 without `--path` and with the main checkout as the root, R2's refusal makes nothing of the verb's own, §3's `recordsRefused(stderr io.Writer) bool`; NICE's section from RR6 review 1, n22 to n27; recordlint holds 26 rows
- [x] check job `20261120-223447-2931` on `c5b2a36` exit 2 on `TestAgentikR1_5_InstallTarget` alone (it pins install's build line, which R1 stamps); mended: the test wants the stamp with plugin.json's version, the packet's may-touch names it
- [ ] `make check` through the gate queue on this commit

## Handover
- If this sitting is cut: the first unticked line above.

## Next act
`make check` through the gate queue.

## Exit
(open)
