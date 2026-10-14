# Packet AR83: turns, time and denials sum over every result event; the cost is the running total

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`'s section "The evidence behind rf1's permission and background-wait items" (2026-10-14), its first item (S3); `design/relay-r1.md` §5 (AR51's one turn count), §7 (the stream parser), §15 (background work); night 2026-10-07-3's row ws4y in the account's ledger and live log (quoted in §1); the code at bce436d |
| pin | `main` at bce436d plus this packet's own commit; a linked worktree, branch `ar83-every-result` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/stream ./internal/night`, `gofmt -l internal/stream internal/night`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/stream/stream.go` (`Tally.Add`'s `result` case, one unexported fold, the `Tally` and `Event` comments only), `internal/stream/ar83_test.go` (new), `internal/effects/testdata/fakeclaude/fixtures/two-results.jsonl` (new), `internal/night/ar83_test.go` (new), `design/relay-r1.md` (§5's "One turn count" paragraph, §7's stream-parser sentence, §11's test list only), `NICE_TO_HAVE.md` (the S3 item, a mark; a new section for what the build leaves), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/stream -run '^TestAR83_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR83_'` reads 2; `go test ./internal/night -run '^TestAR83_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR83_'` reads 2; `go test ./internal/stream -run '^Test(ParseFixtures\|TallyLine\|AR17D_\|AR28_\|AR63\|AR82_\|AR83_)' -count=1 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./internal/night -run '^Test(AR28_\|AR51_\|AR65_\|AR67_\|AR83_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 |
| budget | 60 turns, 150k context, `claude-opus-5-5`; two acts, one commit each |
| verify | `make check` |

## 1. What the source says (quoted)

NICE (S3): "the stream keeps only the last `result` event (`internal/stream/stream.go:233`), so a `claude -p` child that runs a second turn after a background command ends reports that turn alone. Row ws4y (2.1.293) left a `tail -f` in the background; it ended 31 s after the final answer and started a second turn, and status and report show 1 turn, 3.5 s and 0 denials for a 40-turn, 95-minute sitting. Sum the turns, time and denials over every result event, and check whether the spend the night cap reads comes from the last result alone." relay-r1 §15: from 2.1.292 the run waits for a background command and "takes one more turn with the outcome". §5 (AR51): "A sitting's turns are the result's `num_turns`, the number its cost belongs to".

The record (by grep): ws4y's `exit` and `sitting` lines read `turns` 1, `duration_ms` 3455, `denials` 0, `cost_usd` 10.565, beside a `usage` of 7,292,621 cache-read tokens (every message seen); its live log's result lines (`ws4y.log:48`, `:50`) read `turn 47: ... $10.48` and `turn 48: ... $10.57`.

The code at bce436d: the `result` case (`stream.go:230-233`) replaces `t.Result` and `t.CostUSD`; the wire (`:94-121`) has no `duration_api_ms`. Readers: the `exit` line (`internal/night/run.go:449-455`), the gate's exit fact (`run.go:496`), the `sitting` line (`:742`, `:759-765`); through them the tap and `--max-usd` (`night.go:342-351`, `total()` at `:1781`), status's and the report's burn (`status.go:173-191`, `report.go:241`), the sitting notice and a continuation's budget (`wake.go:28-45`).

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar83.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is.

1. **Act 1, the fold.** The `result` case folds each later result into the first, copied as today, instead of replacing it; one result reads as on the pin (`TestParseFixtures` unchanged).
2. **The cost is the highest any result read, never a sum.** `total_cost_usd` is the process's running total: ws4y read $10.48 at its first result and $10.57 one turn later, and $10.565 prices the 7.3 M tokens of all 48 messages. So the cap read ws4y's whole spend; a sum would read $21.05. The highest equals the last under a running total and never reads below an earlier result. `Tally.CostUSD` is `Result.CostUSD`.
3. **Turns, time and denials sum.** `num_turns` and `duration_ms` are per query (ws4y's second read 1 and 3,455 ms); their sum is the count the running cost belongs to, so AR51's pairing holds again; the wait between queries stays in `wall_s` alone. Whether a later `permission_denials` repeats the earlier's the repo cannot settle (ws4y read 0): ruled the conservative reading, the sum, which hides no denial. `duration_api_ms` stays unread (no field, no reader).
4. **The end is the last result's, an error kept.** `ResultTail` and `SessionID` are the last's. `IsError` is true when any result erred, `Subtype` the last erring result's, else the last's: a child cut at `error_max_turns` whose background command then ends takes a `success` turn, and the last alone would pass the gate's exit fact. Account stops already read every erring result (`stream.go:183-193`).
5. **No reader changes.** The `exit` and `sitting` lines read the fold, so every reader in §1 follows; the live log still prints a line per result (`run.go:363-365`).
6. **The fixture**, `two-results.jsonl`: `ok.jsonl`'s 8 lines, its result with `num_turns` 5, one Bash denial, `duration_api_ms` 41000; a top-level text message `msg_04` (input 10, cache read 2000); a second result: `success`, `is_error` false, `num_turns` 1, `total_cost_usd` 1.25, `duration_ms` 3500, `duration_api_ms` 3100, one Bash denial, `result` `the wait ended`. The fold: 6 turns (the tally's 4), 48500 ms, 2 denials, $1.25.
7. **Tests.** `TestAR83_1_TwoResults` (stream): `parseFile` of the fixture: `Result.NumTurns` 6, `DurationMS` 48500, `Denials` 2, `CostUSD` and `Tally.CostUSD` 1.25, `IsError` false, `Subtype` `success`, `ResultTail` the second's text; `Turns` 4, `NonJSON` 0; `Line(130000)` ends `; $1.25`. `TestAR83_2_FoldEdges` (stream, through `Parse`): one result reads as today; $0.05 after $1.20 reads $1.20; `error_max_turns` then `success`: `IsError`, `error_max_turns`, the success's tail; `success` then `error_during_execution`: `IsError`, `error_during_execution`; three results of 2, 1 and 1 turns read 4. `TestAR83_3_SittingLine` (night): `RunRow` as `TestAR51_1` over `w.env("two-results.jsonl")`, `Log` a buffer: the `exit` and `sitting` lines' `turns` 6, `duration_ms` 48500, `denials` 2, `cost_usd` 1.25, `is_error` false; the `exit` line's `subtype` `success`, `result_tail` the second's; gate `green`; the log's two result lines end `$1.20` and `$1.25`. `TestAR83_4_StatusReportCap`: `nightWorld` of r1, r2, r3 on the fixture, `echo ok`, `MaxUSD` 2: two `spawn` lines, the night `budget-reached`, the `stop` line's `cost_usd` and `spent` 2.5; `ReadStatus`: r1 and r2 at 6 turns and $1.25, the burn 2 sittings, 12 turns, $2.50; `BuildReport`'s totals equal it; the Markdown's burn rows read 6 and `$1.25`, the total 12 and `$2.50`. A summed cost would stop after r1.
8. **Act 2, the record.** relay-r1 §5's "One turn count": turns, `duration_ms` and `denials` sum over every result event, the cost is the highest (AR83); §7's sentence: `result` folded over every result event; §11: the four tests. NICE's S3 item gains `(AR83)`.
9. **Left, filed in act 2.** No recorded stream holds two results with a denial, so ruling 3's sum is unproven on a real child, and no line counts a sitting's results: a NICE note, S4.

## 3. Interfaces (signatures, not prose)

`stream`: `func (r *Event) fold(ev Event)`, unexported: `NumTurns`, `DurationMS`, `Denials` added; `CostUSD` the `max`; `IsError` or'd; `Subtype` ev's when ev errs or none has; `ResultTail`, `SessionID` ev's. `Tally.Result` the fold; `Tally.CostUSD` its `CostUSD`. No new exported name, line, key or event.

## 4. Findings that name this row

NICE's S3 item in "The evidence behind rf1's permission and background-wait items" (2026-10-14); relay-r1 §15's background-work bullet, whose re-test stays filed.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory (§1 quotes ws4y's lines). Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: each act's red line; rulings beyond §2; anything filed. Last line: branch, head sha.
