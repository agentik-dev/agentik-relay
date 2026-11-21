# Packet AR51: status and the report count one night the same way

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`, the sightings headed "Status and report disagree" (eight, the last under "The observer's check-in 27"); `design/relay-r1.md` §5 (the `exit` and `sitting` lines) and §9 (status, the report) |
| pin | agent-relay `main` at the commit that lands this packet; worktree `.claude/worktrees/ar51-status-report-one-count`, branch `ar51-status-report-one-count` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state: a real night may be live. No `make check` and no `go test ./...`: `go test ./internal/night` (the whole package once per act, as the regression read), `go test ./cmd/relay -run Report`, `go vet` on those two, `make doclint` and `scripts/hygiene` only, one at a time. **Python:** every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `scripts/hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/night.go`, `status.go`, `report.go`, `run.go` (the `sitting` line's turns only), `internal/night/ar51_test.go`, `internal/night/testdata/` (new fixtures only), `cmd/relay/main.go` (the report's and status's printing only), `design/relay-r1.md` §5 and §9, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else; `testdata/json-keys.golden` does not change |
| acceptance | `go test ./internal/night -run "^TestAR51_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR51_"` reads 5; `go test ./internal/night -run "^Test(Status\|Report\|AR28_\|AR32\|AR38_)" -count=1 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 80 turns, 150k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)
- §9: status is "the dashboard once, from `status.json`"; the report gives "burn per sitting (builders included), branches waiting on a hand merge".
- §5: `exit` carries "(child_exit, subtype, is_error, turns, cost_usd, …)"; `sitting` is "the summary: the stand-in's line as its seed, plus `builders` …".
- The sightings. A live night, one child exited and its gate running: the report prints "0 sittings, $0.00" and status `"sittings":1, "turns":61, "cost_usd":6.69`. An ended night: status 543 turns, the report 446; per row an `exit` line reads 107 and its `sitting` line 79. Status's `builder_output_tokens` is 0 while the report's table sums 10239. Status counts 9 sittings where the report counts 8. A green row's `elapsed` in status grows for hours after its gate.
- The causes, read from the code. Status's burn is an in-memory tap over the night's own appends (`night.go:256-300`, `576-598`; `status.go:137-149`); the report reads ledger lines (`report.go:105-122`). Status counts a sitting at the pick, before staging (`night.go:1110`); the report counts `sitting` lines, written after the gate (`run.go:456`, then `479`, `513`). `exit` takes the result's `num_turns` (`run.go:438`); `sitting` takes the tally of distinct top-level messages (`run.go:691`). `builder_output_tokens` is declared (`status.go:76`) and assigned nowhere. `started[id]` is never closed, so each write recomputes `elapsed` against now (`night.go:583-585`, `682`).

## 2. Rulings
Each is the observer's, 2026-10-02, a reversible choice under entry 15; none is in `DECISIONS.md`.
1. **One turn count.** A sitting's turns are the result's `num_turns`, the number its cost belongs to. The `sitting` line carries the `exit` line's turns; the tally stays the live count on `turn` lines and is the `sitting` line's value only when no result arrived. `TestAR51_1_OneTurnCount`: a fixture stream whose `num_turns` differs from its tally; both lines read `num_turns`.
2. **A sitting counts from its spawn.** One function in `internal/night` derives a night's burn (sittings, turns, cost, builders, builder output tokens) from ledger lines: per row the `sitting` line if written, else the `exit` line, else the `spawn` line with zero cost and the last `turn`. The report calls it; the tap feeds it the same lines, so status's `burn` is its output. A pick that fails to stage is no sitting. `--max-sittings` still compares picks; say so in §9.
3. **The report of a live night says what is in flight.** Its first line reads `N sittings (K in flight), $X: …` when K > 0 and is unchanged when K is 0. `TestAR51_2_LiveReportCounts`: lines `spawn`, `exit`, no `sitting`: the report reads 1 sitting, 1 in flight and the `exit` line's cost, and equals status's burn. `TestAR51_3_BurnAgrees`: after a whole night, `ReadStatus`'s burn equals the report's totals, field by field, `builder_output_tokens` among them and above 0. `TestAR51_4_StageFailureIsNoSitting`.
4. **A row's `elapsed` ends.** It stops at the row's `sitting` line (or its `skip`). `TestAR51_5_ElapsedEnds`: two status writes after the `sitting` line, a clock step between them, read the same `elapsed`.
5. Each test is red on `main` first; the report quotes each red line. Use the package's own shape: `nightWorld`, `runNight`, `w.ledger.Written()`, `night.ReadStatus`, `streamOf`, and literal lines into `BuildReport` as `ar38_test.go:127` does.
6. Out of scope, each its own packet: "waiting on a hand merge" for a row merged by hand, by a later night, or replaced by another row (`report.go:90`, `142-146`); a `revision` key on `night-open`; any new JSON key.
7. §5 and §9 say what rulings 1 to 4 fix, one sentence each. `NICE_TO_HAVE.md` loses the sightings this closes and keeps the hand-merge ones.

## 3. Interfaces (signatures, not prose)
New, unexported or exported as the builder judges: one burn function over ledger lines, returning the fields `status.go`'s burn holds today. Unchanged: every ledger key, every JSON key of `relay status --json`, every stop reason, `BuildReport`'s signature unless the burn function replaces part of its body.

## 4. Findings that name this row
`internal/night/night.go:256-300`, `576-598`, `682`, `1080`, `1110`, `1135`; `status.go:76`, `137-149`; `report.go:15`, `90`, `100-146`; `run.go:438`, `456`, `479`, `513`, `691`; `internal/stream/stream.go:214`; `cmd/relay/main.go:1358`, `1771-1776`; tests `report_test.go:17`, `43`, `ar32b_fix3_test.go:14`, `ar28_test.go:20`, `151`, `night_test.go:36`, `72`, `86`.

## 5. Forbidden reads
Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`; ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)
The table: each acceptance line's number, `make doclint`'s exit and the hygiene scan's last line. Bullets: each test's red line on `main`; the burn function's name and its two callers; what `relay report` prints for the live-night fixture; any existing test whose expectation changed and why; what was left. The charter has its `window` row and its `## Next act`. Last line: branch, head sha.
