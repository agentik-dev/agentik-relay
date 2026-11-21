# Sitting 2026-11-20-2: agentik-relay, rv1-binary-version

| item | value |
|---|---|
| goal | build 2, the fix of review 1: `enabled` takes session's bool spellings (F1); the doctor's `--plugin-root` flag over `CLAUDE_PLUGIN_ROOT` (F2); the `binary` row's ruled states (F3); `build`'s stamp pinned (F5); NICE rows (F6, F4, F7); the packet true to what is built |
| budget | about 90 tool calls, `claude-opus-5-5`; checkpoint at 80% |
| pin | `b281ef7` on main (plugin 0.2.5); build 1 ended at `27de502`, its check on `13259e1` |
| branch | `rv1-binary-version`, worktree `.claude/worktrees/rv1-binary-version`; nothing linked from main |
| level | S3 |
| window | 200k |
| agents | 0 |

## Done-ness
- [x] build 1: R1 to R5 and rr6's review in words; check job `20261120-225308-2932` exit 0 on `13259e1`
- [x] F1: `enabled` known for auto, on, off and the YAML bools true/True/TRUE, false/False/FALSE (effects `TestRV1_5_EnabledSessionBools`); red at `13259e1` on True, TRUE, False, FALSE, green now; effects `ok`
- [ ] F2 and F3: `doctor --plugin-root <dir>` over `CLAUDE_PLUGIN_ROOT`; the row's four states; `/relay:doctor` passes the root
- [ ] F5: the install-line test pins `build`'s stamp too
- [ ] F6, F4, F7: NICE rows
- [ ] the packet true to what is built; the gate's `make check`

## Handover
- If this sitting is cut: the first unticked line above.

## Next act
F2 and F3.

## Exit
Open.
