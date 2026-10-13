# Packet AR58: plan refuses a row whose worktree or branch exists, and a row that never staged is no sitting

| item | value |
|---|---|
| from | `relay-r1.md` §8's rule "`plan` refuses a row whose worktree or branch already exists" (line 192), which `BuildPlan` does not keep, and `--max-sittings`, which counts a row before it stages (§1) |
| pin | agent-relay `main` at the commit that lands this packet; worktree `.claude/worktrees/ar58-plan-sees-the-stage-refusal`, branch `ar58-plan-sees-the-stage-refusal` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state: a real night is live. No `make check` and no `go test ./...`: the acceptance lines, `go vet ./internal/night ./cmd/relay`, `make doclint` and `make hygiene` only, one at a time. **Python:** every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/plan.go`, `internal/night/night.go`, `internal/night/ar58_test.go` (new), existing tests under `internal/night/` and `cmd/relay/` only where a fake leaves a worktree or branch in place that a row then stages over, `design/relay-r1.md` (§8's line 192 and §11's test list only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^TestAR58_" -count=1 -timeout 300s -v 2>&1 \| grep -c "^--- PASS: TestAR58_"` reads 9; `go test ./internal/night -run "^Test(Plan\|Stage\|Schedule\|UntilAndCount\|Report\|AR13_\|AR54_\|AR55_)" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./internal/night -run "^TestAR(1[0-9]\|2[0-9]\|3[0-9]\|4[0-9]\|5[0-7])" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./cmd/relay -run "^TestAR(13\|17C\|32A\|54\|55)_" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 55 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

`design/relay-r1.md` line 192 (§8): "A live claude process whose cwd is inside the repo … `plan` refuses a row whose worktree or branch already exists."

The code keeps that rule only at stage. `internal/night/stage.go:359`: `if fx.FS.Exists(rp.Worktree) { return nil, fmt.Errorf("stage %s: the worktree %s exists", …) }`, and the branch test after it. `BuildPlan` (`internal/night/plan.go:210`) adds no such problem, so a plan reads the row `ready` and the night skips it later, after it has opened, and every row that depends on it is doomed for the night.

`internal/night/night.go:1189-1191`: `count++` runs before `start(id)`, and `start` returns early through `skip` when `Stage` fails (`night.go:722-725`). A row that never staged counts toward `--max-sittings`, and `status`'s `next` reads that count as sittings.

## 2. Rulings

1. **Plan uses stage's own test.** For every row the plan would make `ready` or `pending`, the plan runs the same two tests `Stage` runs: `fx.FS.Exists(rp.Worktree)` and `fx.Git.BranchExists(root, rp.Branch)`. Plan adds no third test, and `Stage` keeps both of its own, since a worktree can appear after plan. A row that fails either test takes a plan problem worded as stage words it: `the worktree <path> exists`, or `the branch <branch> exists`. Like every other plan problem, the problem makes the row `invalid` (`plan.go`'s `StateInvalid`). `run` treats such a row as it treats any invalid row today; this packet adds no new refusal path.
2. A row that is `landed`, `parked` or otherwise not to be started is untouched: a landed row's branch exists by design (`satisfiedHow`, `plan.go:615`). The standing orchestrator row is staged through `Stage` (`night.go:722`), so it takes the check like any row.
3. **A sitting is a row that staged.** `--max-sittings` counts a row only once `Stage` has succeeded for it. A row that `skip`s in `prepare` or at stage frees its count as it already frees its slot. `picked`'s no-progress guard (`night.go:1183`) still ends a loop that picks the same row twice. Decided here because it can be undone: the count measures children spawned, which is what the owner's cap means.
4. `TestAR58_1_PlanRefusesWorktree`: a fake FS that holds the row's worktree path. The row reads `invalid`, its problem is `the worktree <path> exists`, and a row depending on it reads `pending`, not `ready`. `TestAR58_2_PlanRefusesBranch`: the same with the branch. `TestAR58_3_LandedUntouched`: a landed row with its branch merged reads `landed` with no problem. `TestAR58_4_StageFailNotCounted`: `--max-sittings 2` over three independent rows, where the first fails at stage after the plan, through the fake's worktree or lock. The other two both run, and the night's sitting count reads 2.
5. A test that fails after the change because its fake leaves a worktree or branch in place takes a fake without it. No assertion is dropped or loosened. Each changed test is named in the report.
6. `design/relay-r1.md`: line 192 gains one sentence, "the same two tests `Stage` runs (AR58)", and §11 lists the four tests. Nothing else in the design changes.

### Fix round (check-in 46)

Two adversaries found regressions against `main`: a refused row made the plan invalid, so `relay run` refused the whole night. Ruling 1 is replaced, and rulings 7 to 9 are added.

- **Ruling 1, replaced: plan refuses the row, never the night** (§8: "`plan` refuses a row"). A row the plan would make `ready` or `pending` whose worktree or branch exists (`Stage`'s two tests, unchanged) takes plan state `refused` (`StateRefused`) and is never started. Its reason is in stage's words, `the worktree <path> exists` or `the branch <b> exists`, and is a row problem, never a plan problem: the plan stays valid. `relay plan`, the night's `plan` lines (`reason`) and status (`refused: <reason>`) show it from the night's open. The night registers it `skipped`, so its dependents are doomed as a stage-skipped row's are (`schedule.go:70`). It is not measured.
- **Ruling 7 (F0).** The standing orchestrator row is exempt. Its sittings stage `<orc>.<n>` (`SittingPlan`), and `Stage` tests each one. A repo may keep an `<orc>` worktree and branch.
- **Ruling 8 (F1).** At the mid-night re-read, an added row whose worktree or branch exists is refused alone, and the live night goes on.
- **Ruling 9 (F2).** A build row left green and unmerged by an earlier night is refused alone at the next orchestrated night's plan, and that night runs.

Tests: `_1` and `_2` read the plan valid, the row `refused` with its reason, and the dependent `pending`, doomed at run. New are `TestAR58_5_OrchRowExempt`, `TestAR58_6_RereadRefusesRowOnly` and `TestAR58_7_GreenUnmergedRefusedAlone`. The fix round's acceptance widens lines 2 to 4 to take in `UntilAndCount`, `TestAR1x` and `TestAR2x`, and `cmd/relay`'s `AR54` and `AR55`. `may touch` widens to cover `internal/night/*.go` for the per-row state, and to the tests that pin a state list.

Hardening round (round-2 verify): from the night's open on, a refused row is exactly what a stage-skipped row was on `main`. Its `skip` line is written when it is registered, in stage's words, so the report lists it. Status shows its worktree, `schedule.go` dooms it behind a failed dependency so the stop and exit match `main`'s, and `TestAR17C_1_SignalMidPlan`'s fixture leaves no branch `r`. New are `TestAR58_8_RefusedReportsAsSkipped` and `TestAR58_9_RefusedStopMatchesMain`.

## 3. Interfaces (signatures, not prose)

`func TestAR58_1_PlanRefusesWorktree(t *testing.T)`, `func TestAR58_2_PlanRefusesBranch(t *testing.T)`, `func TestAR58_3_LandedUntouched(t *testing.T)`, `func TestAR58_4_StageFailNotCounted(t *testing.T)`. Reuse the fakes in `internal/night/plan_test.go` and `night_test.go`; add none in `effects`.

## 4. Findings that name this row

The observer, check-in 46 (2026-10-04, 18:37). A target repo's night opened at 00:48:03Z. Its plan lines read rows es4 and fen2b `ready`, though their worktrees were left from an earlier night that an API outage stopped. Stage then refused both (`stage es4: the worktree …/.claude/worktrees/es4 exists`), and their dependents esfix8 and fenfix2 were doomed for the night. `status` read `count-reached after 10 sittings (3 so far)` while one sitting had run. `NICE_TO_HAVE.md` line 199 is a related case: a green unmerged row is skipped at stage by the next night. This packet makes that case `invalid` at plan, and leaves its re-gate to that line.

## 5. Forbidden reads

Transcripts; `~/.local/state/agent-relay`. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line with its count, `go vet`'s exit, `make doclint`'s exit and `make hygiene`'s last line. Bullets: which TestAR58 tests were red on `main` and why; each existing test changed and the line it changed; anything filed. Last line: branch, head sha.
