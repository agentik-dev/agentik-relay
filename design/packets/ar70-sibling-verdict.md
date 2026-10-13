# Packet AR70: a sibling killed in its gate lands its verdict

| item | value |
|---|---|
| from | `design/usage-limit-resume-r1.md` §2 (a sibling killed in its gate), §4 test 4 (its sibling and "spawned apart"), §5 packet 5's rest; `NICE_TO_HAVE.md`'s AR64 (N4), AR65 (probe B, the broken line), AR67 (the uncollected outcome) and AR69 notes; `design/relay-r1.md` §1, §11 |
| pin | agent-relay `main` after AR69's merge (AR69 moves `night.go`; this row depends on AR68 and AR69); worktree `.claude/worktrees/ar70-sibling-verdict`, branch `ar70-sibling-verdict` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet ./internal/night ./internal/relay`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/relay/machine.go` (the `Paused` row and its comment only), `internal/relay/machine_test.go` (`TestStepMatrix`'s legal map only), `internal/relay/ar64_test.go` (`TestAR64_1`'s refusals only), `internal/relay/ar70_test.go` (new), `internal/night/night.go`, `internal/night/ar67_test.go` (`TestAR67_5_NoHeldStage` and its comment only), `internal/night/ar70_test.go` (new), `design/relay-r1.md` (§1, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/relay -run "^Test(AR70_\|AR64_\|StepMatrix)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR70_\|ok )"` reads 2; `go test ./internal/night -run "^Test(AR70_\|AR69_\|AR68_\|AR67_\|AR66_\|AR65_)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR70_\|ok )"` reads 4 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The record, §2: "A sibling killed in its gate (`night.go:794-800` keeps it in `running`) is gated again at the wake, never given a child." "Each paused row keeps its `Staged` and its stage lock". §4 test 4: "both continue, spawned apart." AR64's N4: "A sibling gated again at the wake is stepped from a literal `Exited` (`RunRow`), so §1's table never shows paused to green, red or failed." AR65's: a sibling killed in its gate "writes its line unheld and unlocks its stage" (probe B); "A paused night does not stop invariant on a killed sibling's broken line". AR67's: a sibling "still uncollected at the pause is stopped as 'killed in its gate' even when it was not in its gate".

The code at `dc6bb83` (AR68's head; AR69 moves `night.go`): the gate reads no context (`cmd/relay/main.go:1313-1316`); the pause's kill cancels the row's context and signals the child (`internal/night/night.go:220-233`), and `RunRow` reads that cause before the gate (`internal/night/run.go:438-447`), so a part killed after its child's exit runs its gate to the end, holds no line and unlocks (`night.go:767-771`); `collect` drops a paused row's outcome with no held line (`:1072-1080`); a paused row's `accountBy` fails any outcome (`:1131`); the wake stops `killed in its gate` (`:1347-1359`); `Paused` takes `spawn` and `halt` only (`internal/relay/machine.go:14`).

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar70.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message.

1. **The verdict, not a second gate.** A sibling killed in its gate, like one whose outcome waited uncollected, holds its gate's whole verdict on a tree no child touches since; a second gate on that tree and cut reads the same. It lands that verdict where its outcome is collected, paused or not, never gated again nor given a child; a paused night takes it as it takes a row outside a model's scope (relay-r1 §1). The record's "gated again at the wake" and probe B's lock serve a row that continues; relay-r1 §1 records this as an amendment to the record's §2.
2. **Act 1, the pair** (`relay`). `Paused` takes `gate-green` → green, `gate-red` and `gate-unrun` → red, `gate-failed` → failed, as `Exited` does. `TestStepMatrix`'s legal map gains the four (still 11×16 pairs); `TestAR64_1` refuses every other event.
3. **Act 2, the landing** (`collect`). A paused row's late outcome with no held line: its `accountBy` mark deleted, `stepSitting(Paused, …)` by its state (green `gate-green`, red `gate-red`, failed `gate-failed`), then on as any outcome: its own reason kept (`auth`, `invariant`, `orphans`), a green merged under the policy, its lines printed, the night stepped (green and red keep a paused night paused; a failure stops it with its reason, `auth` killing the running rows as today). A skipped outcome stays paused for the halt.
4. **Act 2, a broken held line.** AR69's held branch, the line written paused, its invariants broken (`o.s.Reason` not `usage-limit` after the write): `Paused` steps `halt`, failed `invariant`, no held stage kept, the stage unlocked, printed with its resume line, the night `OnSittingFailed` `invariant`; the halt follows.
5. **Act 2, the wake.** A paused row whose outcome is still out (in `running`: its gate outlasts the pause), the orchestrator's sitting included, is neither continued, halted nor checked; the `wake` line names it, and it lands when collected (ruling 3). AR67's `killed in its gate` stop goes. The halt is unchanged: the loop collects every outcome before it.
6. **Act 2, the test the landing breaks.** `TestAR67_5_NoHeldStage` reads r2 green from its one `sitting` line, the `wake` line `[r1]`, r1's part 2, both green, `stopped:queue-empty`, 3 spawns; its comment follows.
7. **Tests.** relay: `TestAR70_1_PausedVerdict` (the four pairs; `spawn` and `halt` as before; every other event refused). night, on TestAR67's world (`--parallel 2`, r1 `ar67Limited`): `TestAR70_2_GateOutAtTheWake` (TestAR67_5's r2 gate held until the `wake` line: that line `[r1 r2]`, a part 2 for r1 only, r2 green after it, unlocked, `queue-empty`, 3 spawns). `TestAR70_3_SiblingStops` (r2 a stall the pause kills, the clock never moved; its stream at the kill an `is_error` result `API Error: 401`: `stopped:auth`, r2 failed auth; r2's `Deps` `{"zz": false}`: `stopped:invariant`, r2's one `sitting` line `reason` invariant, no `limit`; each: r1 failed usage-limit, both unlocked, trees kept, no `wake` line). `TestAR70_4_SpawnedApart` (TestAR67_3's world, `fx.Load` a quiet `flipLoad` as TestAR13_5's: after the wake one part 2 spawns, the second SpawnSpacing later and not a second sooner; both green, `queue-empty`).
8. **The record, act 3.** relay-r1 §1: the pair, the landing and its amendment, the broken line, the wake's skip (replacing "a paused row's late exit is discarded" and the `killed in its gate` stop); §11: the tests. The NICE_TO_HAVE notes this row takes gain `(AR70)`.
9. **Left.** Two edges go to NICE_TO_HAVE: a kill landing after the child's exit but before `RunRow` reads its cause continues the row into a part 2 with its acts done; a held outcome collected after the wake (a kill outlasting the 2-minute slack) waits until the halt. AR71 takes a model's scope; packet 6 the readers; `Night.tla` stays out.

## 3. Interfaces (signatures, not prose)

`relay`: `step[Paused]` gains `GateGreen: Green, GateRed: Red, GateFailed: Failed, GateUnrun: Red`. `night`: no new name. Red on the pin: `_1` (`Step(paused, gate-green)` refused); at act 1's head, `TestAR67_5` as ruled and `_2` (`r2: killed in its gate`), `_3` (the night stays paused: the 20-s wait fails); `_4` pins test 4's sentence and may pass once written.

## 4. Findings that name this row

AR69 ruling 8 names AR70 (the sibling's verdict). AR64's N4, AR65's probe B and broken line, AR67's uncollected outcome: section 1.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: the TestAR70 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
