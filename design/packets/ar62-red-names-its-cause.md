# Packet AR62: a red row names its cause

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`'s check-in 11 and check-in 58 entries (`reason":""`); the ledger of night 2026-10-10-3, row gguest; `design/relay-r1.md` §5 and §9 (`status`, `report`) |
| pin | agent-relay `main` after AR60's and AR61's merges (both move `night.go`; AR60 `status.go` and `report.go`); worktree `.claude/worktrees/ar62-red-names-its-cause`, branch `ar62-red-names-its-cause` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet` of the four packages, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/relay/gate.go` (`Gate.Dirty` only), `internal/gate/gate.go`, `internal/gate/ar62_test.go` (new), `internal/night/run.go`, `internal/night/night.go` (`collect` only), `internal/night/status.go`, `internal/night/report.go`, `internal/night/ar62_test.go` (new), `internal/night/testdata/json-keys.golden` (ruling 4's two keys), `design/relay-r1.md` (§5, §9, §11 only), `plugin/agents/attendant.md` (one bullet), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/gate -run "^Test(AR62_\|AR45_\|AR59_3_\|TouchSet\|GateGreen)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR62_\|ok )"` reads 3; `go test ./internal/night -run "^Test(AR62_\|AR45_\|AR59_3_\|AR47Fix_Golden\|StatusResumeLines\|Report\|AR30_3_\|NightEndToEnd)" -count=1 -timeout 600s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR62_\|ok )"` reads 3; `go test ./cmd/relay -run "^Test(JSONShapesFrozen\|NightVerbs)$" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./cmd/relay -run "^TestAR59_4_Gofmt" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The ledger, night 2026-10-10-3, row gguest, 2026-10-11T04:17:18Z: every acceptance line and the verify line green, then `{"event":"touch","paths":["internal/daemon/gateguest_test.go"]}` and the end line `{"event":"gate",…,"reason":"","result":"red"}`; the `sitting` line's gate reads `"red":["touch"]` (AR45). Check-in 11 (g2b): "the red check was `touch` (ten paths outside the packet's `may touch`), which only the separate `gate` and `touch` events carry." Check-in 58 (gacc, the hand gate fen3b): "The gate's end line reads `"result":"red","reason":""`". AR59 named the cut; every other red still reads `""`.

The code at `1cb540e` (AR60 and AR61 move the lines): `relay.Decide` returns `RedResult, ""` for touch, off-branch, packet-differs and orphans (`internal/relay/gate.go:166`), a red line (`:171`), a red verify (`:175`). The end line is `end := map[string]any{"result": …, "reason": string(why)}` (`internal/gate/gate.go:1186`); `Red` (`:1073`) names the checks, `init` hands it to `night.RedChecks` (`:1096`); `gather` keeps no count (`g.Clean = len(status) == 0`, `:304`). The `sitting` line's gate object is result, verify, red (`internal/night/run.go:682`). `setRow` copies state and reason (`night.go:706`); the live line is `relay <id>: <state>` (`night.go:1058`). `PrintStatus` writes `state: reason` (`status.go:284`); `BuildReport` takes the `sitting` line's `reason` (`report.go:122`), `Markdown` writes `, reason` (`report.go:407`).

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar62.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`; its red line goes in the commit message.

1. **Act 1, the cause.** `gate.Cause(g)` names each check `Red(g)` names, in its order, once, with its first fact, joined by `; `: `branch: off the row's branch`; `exit: no result event` or `exit: is_error`; `commit: none`; `clean: <n> paths` (`Gate.Dirty`, the count `gather` read; `1 path`); `origin: a remote moved`; `touch: <first> outside may touch (+<n> more)`; `charter: none`; `queue: invalid at the head`; `merges: unproven`; `packet-source: differs at the head`; `acceptance <n>: last "<last>", want <expect>` for the first failing line (`acceptance <n>: unrun` when unrun), `(+<n> more)` for the others; `verify: exit 124, cut at <n> s` when cut, else `verify: exit <e>, last "<last>"` or `verify: unrun`; `main-moved: main at <sha12>`; `orphans: <n> swept`. No `(+0 more)`. A path or last line is clipped to 60 runes and the line to 200 by a copy, in gate.go, of `night`'s unexported `clip` (`plan.go` is outside may touch). Never an acceptance line's shell (`TestGateGreen`). `""` when `Red` names none.
2. **Act 1, the end line.** It gains `cause` when `Cause(g)` is not `""`. `reason` stays the stop reason, §1's names that `StopReasons`, the machine and the notices read; a sentence there is no reason. `detail` stays the error's or the cut's (AR59).
3. **Act 2, the sitting and the row.** The gate's `init` sets `night.RedCause` to `Cause`, beside `RedChecks`; the `sitting` line's gate object gains `cause` (`""` when none or unlinked, never absent), with `orphans` the sweep added. `RowStatus.Cause` is the gate's cause when the row ends red or failed, else `""`; `collect` sets it under `lg.mu` beside `setRow` and prints `relay <id>: cause: <cause>`, clipped under 100 columns, after its `relay resume <id>` line and only when the cause is not empty (`TestAR31_2_NightNamesLog` and `TestAR5_NightLinesResumeAndTilde` want the state, log and resume lines adjacent). A row merged by hand keeps it, as its reason (AR56).
4. **Act 2, the readers.** `PrintStatus` writes `cause of <id>: <cause>` for each row with one, after the acts and before `burn:`, clipped under 100 columns; `--json` carries it whole. `StoppedRow.Cause` is the `sitting` line's `gate.cause`; `Markdown` writes `- <row>: <state>[, <reason>] (<cause>)`, the line clipped under 100 columns. The notice reads `gate.result` alone, so no path leaves. `json-keys.golden` gains `report stopped.rows.[]cause` and `status rows.[]cause` alone (`UX1_GOLDEN=1`).
5. **Tests.** gate: `TestAR62_1_Cause` (over `relay.Gate` values: each fact above; ungathered and green `""`; three outside paths `(+2 more)`; red lines 3 and 5 `acceptance 3: … (+1 more)`; clean before touch; a 300-rune last clipped, the line ≤ 200 runes); `TestAR62_2_EndLineCause` (`TestTouchSet`'s world: `reason` `""`, `cause` `touch: b.txt outside may touch`; `TestGateGreen`'s: no `cause`; an untracked file: `dirty-tree`, `clean: 1 path`). night: `TestAR62_3_SittingCause` (`ar45Row`: outside, the same cause in the gate object and the end line; green `""`; `TestAR45_2`'s row names clean, touch, acceptance in order); `TestAR62_4_StatusAndReport` (`TestStatusResumeLines`'s night, `Out` a buffer: r1's status `cause` holds `acceptance 1: last "no"`, r2's `""`; `PrintStatus` holds `cause of r1: acceptance 1:`, each line under 100 columns; `Out` holds `relay resume r1\nrelay r1: cause: `; `BuildReport`'s stopped r1 carries it; `Markdown` holds `- r1: red (acceptance 1:`).
6. **The record, in act 2.** §5: the two `cause` keys; §9: `status` and `report` show it; §11: the four tests. AR60 rewrites those §9 rows and appends to §11, as AR61 does to §11: edit them by content after the merge. `attendant.md`: a red row's `cause` before its `gate` and `touch` lines. The check-in 11 and 58 entries gain `(AR62)`.
7. **Not done.** `Decide` and the reasons; `relay gate`'s text, which prints each fact, and its `--json`; `Check.Cut`, `Gate.VerifySeconds` and `PacketDiffers` stay out of `--json` (ar59-2).

## 3. Interfaces (signatures, not prose)

`gate`: `func Cause(g relay.Gate) string`. `relay`: `Gate.Dirty int` (`json:"-"`). `night`: `var RedCause func(relay.Gate) string`, `RowStatus.Cause string` (`json:"cause"`), `StoppedRow.Cause string` (`json:"cause"`). Tests `func TestAR62_<n>_<Name>(t *testing.T)`; red on `main`: each (undefined names).

## 4. Findings that name this row

§1's three: gguest (2026-10-10-3), g2b (2026-09-30-8), gacc and fen3b (2026-10-06). Each time the attendant read the cause from the separate `touch` and `gate` lines.

## 5. Forbidden reads

Transcripts; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line with its count, `go vet`'s exit on the four packages, `make doclint`'s exit and `make hygiene`'s last line. Bullets: the TestAR62 tests red on `main` and why; each existing test changed; anything filed. Last line: branch, head sha.
