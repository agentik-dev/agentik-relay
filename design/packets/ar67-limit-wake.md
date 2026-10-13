# Packet AR67: the wake continues the row in its tree

| item | value |
|---|---|
| from | `design/usage-limit-resume-r1.md` §5 packet 4, its first half (§2's wake and tree check; §4 test 1's wake); `NICE_TO_HAVE.md`'s `ar64-r1` and N9; `design/relay-r1.md` §1, §5 |
| pin | agent-relay `main` at `4814043` (AR60 to AR66 merged); worktree `.claude/worktrees/ar67-limit-wake`, branch `ar67-limit-wake` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet ./internal/night ./internal/effects`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/wake.go` (new), `internal/night/night.go`, `internal/night/run.go` (`RunOptions` and `appendLine` only), `internal/effects/fake.go` (`FakeSpawner` and `fakeProcess` only), `internal/night/ar67_internal_test.go` (new), `internal/night/ar67_test.go` (new), `internal/night/ar65_test.go` (`TestAR65_5_WakeHalts` and its comment only), `internal/effects/ar67_test.go` (new), `design/relay-r1.md` (§1, §5, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^Test(AR67_\|AR65_\|AR66_\|AR63_3_\|AR17D_0_\|NightEndToEnd\|RunOneRow)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR67_\|ok )"` reads 5; `go test ./internal/effects -run "^Test(AR67_\|FakeSpawner)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR67_\|ok )"` reads 2; `go test ./cmd/relay -run "^TestAR59_4_Gofmt" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The record, §2: "The night steps `OnWake`, writes a `wake` line and checks `--until` and `--max-usd` [...], not `--max-sittings` [...]. A failed check stops the night and halts the paused rows. Else each paused row, in queue order, gets a continuation": "the worktree is on the row's branch and its HEAD descends from the stage's `head_before`; else the night stops `usage-limit`, `<row>: the tree moved while paused`"; "a new session id; `--max-turns` and `--max-budget-usd` are the row's less its parts' tonight, never under 10 turns or $5"; "`RunRow` on the first stage's `Staged`, so the gate judges every part against one cut; `part` on its lines". "A sibling killed in its gate [...] is gated again at the wake, never given a child." "A continuation is not a pick". `ar64-r1`: "at the wake, the `--max-sittings` poll [...] can stop the night before any continuation starts."

The code at `4814043`: the paused poll stops `woke at <wake>` (`internal/night/night.go:1257-1272`); `start` (`:720-806`); `heldSt` is set only for a held line (`:1013-1019`); the pause sets `accountBy` (`:1046`), never cleared, and `accountBy` fails an outcome (`:1071-1084`).

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar67.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message.

1. **Act 1, `Outs`.** `FakeSpawner.Outs []FakeRun`: the n-th `Start` gets `Outs[n]`; past the last, `Out` and `Exit` as today.
2. **Act 1, the budgets** (`wake.go`, pure): `remainder` is the queue row's `turns` and `usd` less its parts' tonight (each `sitting` line's `turns`; its `cost_usd`, or its `spawn` line's `usd` when `cost_read` is false), at least 10 and $5, dollars rounded down. `continueArgv` sets `--session-id`, `--max-turns`, `--max-budget-usd`, the rest the plan's.
3. **Act 2, the wake.** The paused poll steps `OnWake`, writes a `wake` line (`rows` sorted, `wake`, `count` the night's pauses; no notice), then `--until`, then `--max-usd`: either stops, and AR65's halt follows. Not `--max-sittings` (`ar64-r1`); a continuation adds nothing to `count`.
4. **Act 2, the checks**, every paused row before any continues. A row with no held stage (a sibling killed in its gate) gets no child: `OnSittingFailed` `usage-limit`, detail `<row>: killed in its gate` (packet 5 gates it again). Else its tree's `Branch` is `Staged.Branch` and `IsAncestor(HeadBefore, Head)`; an error or a false: likewise, `<row>: the tree moved while paused`. A paused orchestrator sitting: likewise, `<id>: an orchestrator sitting is not continued` (AR68 replaces it). Each stop halts every paused row.
5. **Act 2, the continuation**, in queue order: `Step(Paused, Spawn)`; its `accountBy` and `killedBy` entries deleted; the stdin and `RunOptions` `start` rendered, kept before it sets `Ctx`, `hold` and `Log`, each part getting its own (the row's log reopened); a copy of the row's plan with a new session (`PlanOptions.UUID`, else `NewUUID`) and `continueArgv`'s argv; `RunRow` on the `Staged` taken from `heldSt`: no `Stage`, no `stage` line, no window. `RunOptions.Part` numbers the part; from 2, each line through `appendLine` carries `part`. Limited again, it pauses again.
6. **Act 2, `picked`** is unchanged: a continuation never passes `PickIn`, so no-progress never reads it.
7. **Act 2, the test the wake breaks.** `TestAR65_5`'s case `the wake` becomes `--until 02:05`, the clock to 02:05: `stopped:until-reached`, r1 halted as asserted, one spawn; its comment follows.
8. **Tests.** `TestAR67_1_FakeOuts` (two runs, then `Out`). Internal `TestAR67_2_Budgets` (100 turns, $15 less 40 turns, $11.5: 60 and 5; less 95 turns: 10; an unread part at its usd; only the three argv values change). Nights on `TestAR65_3`'s world, r1's `Outs`: the refused `five_hour` event (`resetsAt` T+1h), an `is_error` result `API Error: 429` (`num_turns` 40, `total_cost_usd` 11.5), exit 143; then a clean result, exit 0; the spawner commits `a.txt` at each part 2. `TestAR67_3_WakeContinues` (`--parallel 2`, r2 a stall the pause kills): paused, the test commits `a.txt` in r1's tree; at the wake one `wake` line (`[r1 r2]`, T+1h+2m, 1); r1's part 2 in its tree, a new `--session-id`, `--max-turns 60`, `--max-budget-usd 5`; one `stage` line a row; each part 2's `spawn` and `sitting` lines `part` 2, the latter's `head_before` the `stage` line's; r1 and r2 green, `stopped:queue-empty`, unlocked; with `--max-sittings 2`, both part 2s run, then `count-reached`. `TestAR67_4_TreeMoved` (r1's tree on a new branch while paused: `stopped:usage-limit`, that detail, one spawn, r1 failed, unlocked, its tree there). `TestAR67_5_NoHeldStage` (`--parallel 2`, r1's stream sent once r2's gate starts, that gate held until r1's pause line: at the wake `stopped:usage-limit`, `r2: killed in its gate`, no part 2).
9. **The record, act 3.** relay-r1 §1: `wake` stepped, the continuation, no-progress's exception, the stops at the wake; §5: the `wake` line, `part`; §11: the tests. `ar64-r1` and N9 gain `(AR67)`.
10. **Not done.** The RESUME block, the start hook's cut, the orchestrator sitting at the wake, `nextSitting`'s red list, `orc.stuck` (AR68); the guards, the sibling gated again (packet 5); the readers (packet 6). AR65 to AR67 install only with AR68.

## 3. Interfaces (signatures, not prose)

`effects`: `type FakeRun struct { Out []byte; Exit int }`. `night`: `func remainder(turns, usd int, parts []map[string]any) (int, int)`, `func continueArgv(argv []string, session string, turns, usd int) []string`, `RunOptions.Part int`. Red on the pin: `_1`, `_2` (undefined); at act 1's head, `_3` (the wake stops `woke at`, no part 2), `_4` and `_5` (the detail `woke at`).

## 4. Findings that name this row

The record's §1: five ledger hits, each a night stopped `usage-limit`. The owner (observer, 2026-10-10): "sessions and relay runs resume by themselves after a usage limit (the 5-hour window), instead of stopping."

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line with its count, `go vet`'s exit, `make doclint`'s exit and `make hygiene`'s last line. Bullets: the TestAR67 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
