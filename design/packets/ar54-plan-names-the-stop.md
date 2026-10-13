# Packet AR54: a plan that leaves checks past the budget says the run will stop

| item | value |
|---|---|
| from | the ledger of night 2026-10-02-2 (§1); `NICE_TO_HAVE.md`, the item "Night 2026-10-02-2 stopped `measure-timeout`" |
| pin | agent-relay `main` at the commit that lands this packet; worktree `.claude/worktrees/ar54-plan-names-the-stop`, branch `ar54-plan-names-the-stop` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state: a real night may be live. No `make check` and no `go test ./...`: the acceptance lines, `go vet ./internal/night ./cmd/relay`, `make doclint` and `make hygiene` only, one at a time. **Python:** every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/plan.go` (`Plan`, `BuildPlan`'s end, `Print`), `internal/night/night.go` (the `measure-timeout` stop's detail, ruling 2 only), `internal/night/ar54_test.go` (new), `design/relay-r1.md` (the "Bounded" item's sentence on `Plan.Budget`), `plugin/commands/plan.md` (one sentence), `internal/night/testdata/json-keys.golden` (ruling 5 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^TestAR54_" -count=1 -timeout 300s -v 2>&1 \| grep -c "^--- PASS: TestAR54_"` reads 3; `go test ./internal/night -run "^TestAR38_" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./internal/night -run "^TestAR34_" -count=1 -timeout 600s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./cmd/relay -run "^TestJSONShapesFrozen$" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 40 turns, 150k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

Night 2026-10-02-2 (example-app): the session ran `relay plan`, and at 22:33:29Z its `plan` lines read `"measure_seconds": 600, "measure_from": "default", "budget": 3`. The plan had already cut three checks past the budget. Its text output says so only at the end of its last line (`measured 24 lines, skipped 8, 3 past the budget`), and its `--json` says so only as `"budget": 3`. The session started `relay run` with no `--measure-seconds`. At 22:44:03Z that night's plan read `"budget": 10`, and the run stopped at once with `"reason": "measure-timeout"` and the detail "10 checks unrun: the measure ran out of time (600 s, from default); raise measure_seconds in the queue or pass --measure-seconds". $0 was spent and ten minutes were lost. The next night set the measure to 2400 and measured in 602 s.

`relay run` treats a plan with `Budget > 0` as a stop (`internal/night/night.go:590-592`). `relay plan` treats the same plan as a footnote. A session that reads the plan's JSON has no field that says the run will stop.

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar54.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. `DECISIONS.md` stays as it is. Each act's test is red on `main`; put the red line in the commit message.

1. **The plan names the stop (act 1).** `Plan` gains `RunStop string` with the JSON key `run_stop`. It is always present and `""` when a run on this plan would not stop for it. At `BuildPlan`'s end, when `p.Budget > 0`, it is `"measure-timeout: " + MeasureTimeoutDetail(p)`. `Print` writes `  a run on this plan stops at once: <RunStop>` on its own line, directly after the queue problems and before the first row. The last line's `, K past the budget` stays. The exit code does not change (portfolio entry 25: an exit code reports the task). `internal/night/ar54_test.go`: `TestAR54_1` builds the AR38 fixture's plan (`q.Defaults.MeasureSeconds = 600`, a clock that steps 250 s a line) and reads `RunStop`, the printed line, and `"run_stop"` in `json.Marshal(p)`; `TestAR54_2` builds a plan whose measure fits and reads `RunStop == ""` and no such line.
2. **One wording (act 2).** The run's `measure-timeout` stop detail is `RunStop` without its `measure-timeout: ` prefix: read it from the plan rather than computing it a second time. `TestAR54_3` runs the AR38 fixture and reads the `stop` line's detail as exactly `strings.TrimPrefix(n.Plan.RunStop, "measure-timeout: ")`.
3. **The records (act 3).** `design/relay-r1.md`'s "Bounded" sentence on `Plan.Budget` gains "and `run_stop` names the stop a run on it makes". `plugin/commands/plan.md` gains one sentence: a non-empty `run_stop` means `relay run` on this plan stops at once; raise the measure first. `NICE_TO_HAVE.md`'s item leaves with "(landed as AR54)".
4. **What this packet does not do.** It does not change the budget, the memo, the cut, the ledger's keys or a stop reason. It does not make the plan invalid or make it measure again. It does not guess a budget that would fit.
5. **The review's fix round (act 4).** The review's F1, 2026-10-03: `TestJSONShapesFrozen` (`cmd/relay/main_test.go:652`) freezes `relay plan --json`'s keys, and `internal/night/testdata/json-keys.golden` has no `plan run_stop`. Regenerate the golden through the test's own switch (`UX1_GOLDEN=1`), and check that the only change is that one line. Its F3: a plan with no ready row has `run_stop` `""`, yet the run stops `gate-failed-dependency`. So `RunStop`'s doc comment, `design/relay-r1.md` and `plugin/commands/plan.md` say `run_stop` names the stop the measure's budget makes, and `""` means the budget cut nothing, not that the run will start. Add the review's F2 and F4 to `NICE_TO_HAVE.md` as the hand-off gives them.

## 3. Interfaces (signatures, not prose)

New: `Plan.RunStop string` (`json:"run_stop"`). Unchanged: `BuildPlan`, `Print`, `MeasureTimeoutDetail`, `MeasureLine`, and every ledger line's keys.

## 4. Findings that name this row

`internal/night/plan.go:44-77` (`Plan`), `:204` (`BuildPlan`), `:389` (`p.Budget = budgetCut(p)`), `:953-1016` (`Print`; the trailer at `:1007-1011`); `internal/night/night.go:590-592` (the run's stop), `:835-841` (the orchestrator's plan check); `internal/night/bound.go:205-211` (`MeasureTimeoutDetail`); `internal/night/ar38_test.go:72-111` (the fixture to copy); `design/relay-r1.md:496`.

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line with its count, `go vet`'s exit, `make doclint`'s exit and `make hygiene`'s last line. Bullets: each act's red line on `main`; the printed line's words; what was left. Last line: branch, head sha.
