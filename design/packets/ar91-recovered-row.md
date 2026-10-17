# Packet AR91: a recovered row is not `stream-error`; the child's end is its last result's, a cap kept, the erred results counted

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`, "AR83's review (2026-10-14, the observer; two adversaries PASS, 0 findings)", its S3; `design/relay-r1.md` §1, §5, §6, §14 (D19); the survey `research/synthesis/tools-relay.md` in agentik-research, §2.5 and §2.6 (quoted in §1); the code at 8505f11 |
| pin | `main` at 8505f11 plus this packet's own commit; a linked worktree, branch `ar91-recovered-row` |
| wall | AR83's: this repo only; no push, `--amend`, rebase, device, ssh or network; no `relay` binary by hand or outside `make`'s harness; no `relay` or `agentik relay` verb on the account's state; never `claude`. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet` and `gofmt -l` of `internal/stream` and `internal/night`, `make doclint`, `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Never spell the Go name of the local zone. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/stream/stream.go` (`Tally.Errors`, `Add`'s `result` case, `fold`'s end, the `Tally` and `fold` comments only), `internal/stream/ar91_test.go` (new), `internal/effects/testdata/fakeclaude/fixtures/recovered.jsonl` (new), `internal/night/run.go` (the two `exit` lines' `errors` only), `internal/night/ar91_test.go` (new), `design/relay-r1.md` (§5's `exit` keys and "One turn count", §11's test list only), `NICE_TO_HAVE.md` (the S3 item, a mark; one new section), `history/sittings/*.md`, `history/sessions/NEXT`. Nothing else |
| acceptance | `go test ./internal/stream -run '^TestAR91_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR91_'` reads 1; `go test ./internal/night -run '^TestAR91_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR91_'` reads 2; `go test ./internal/stream -run '^Test(ParseFixtures\|TallyLine\|AR83_\|AR86_\|AR90_)' -count=1 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./internal/night -run '^Test(FailedStops\|WallClockKill\|AR10_7_\|AR51_\|AR83_\|AR90_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1; `grep -c -E '"type":"result"' internal/effects/testdata/fakeclaude/fixtures/recovered.jsonl` reads 2; `grep -c -E '"is_error":true' internal/effects/testdata/fakeclaude/fixtures/recovered.jsonl` reads 1 |
| budget | 60 turns, 150k context, `claude-opus-5-5`; three acts, one commit each; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says (quoted)

NICE (S3): "ruling 4 (`is_error` if any result erred) fails a row `stream-error`, which stops the night (`night.go:28`), so a transient non-account API error followed by a successful second result now stops the night where `main` went green; the packet rules it but does not name the stop, and no night-level test covers an erring first result then a success."

The record. relay-r1 §1: `stream-error` "(is_error, no result event, spawn failure)" is a `failed` reason; "A sitting failure (`failed`) says the prompt or the driver is broken; the night stops with that reason" (D19, run 10 ruling 2). §14: "D19 sitting failures stop the night, gate reds move on", "ruled by the owner 2026-09-23". §6: "`exit`: a result event was seen and `is_error` is false; else `failed: stream-error`." §5 (AR83): "an error is kept (`is_error` when any result erred, `subtype` the last erring result's, else the last's)". AR83 ruling 4's reason: "a child cut at `error_max_turns` whose background command then ends takes a `success` turn, and the last alone would pass the gate's exit fact." The survey reads the same (§2.5: "`exit`: a result event was seen and `is_error` is false; else failed `stream-error`"; §2.6: a sitting failure "means the prompt or the driver is broken, and the night stops with that reason").

The code at 8505f11: `fold` ors `IsError` (`internal/stream/stream.go:321-324`); `Add`'s `result` case (`:272-284`); `Account` skips a cap (`account.go:53`); the `exit` lines (`internal/night/run.go:627`, `:831-837`); the gate's exit fact (`run.go:878`, `internal/gate/gate.go:248`, `internal/relay/gate.go:161-162`); the stop (`internal/relay/night.go:28-32`, the `night.go:28` NICE cites; `SittingFailures`, `types.go:215`).

No entry of this repo's `DECISIONS.md` rules the exit fact or the night's stop; D19 does, beside portfolio entry 23.

## 2. Rulings

Standing rules. The charter, `history/sittings/<date>-<n>-ar91.md`, opens at act 1 and closes before the last commit; no home path. Each act's first test is red on the pin, its red line in the commit message. `DECISIONS.md` stays as it is.

1. **The end is the last result's.** A child's state is the one it ended in: `IsError` and `Subtype` are the last result's. One case is kept, AR83's own: an erring cap (`is_error` true, `subtype` `error_max…`, the sitting's own turn or budget cap, as `Account` reads it) holds through every later result, its subtype with it; a wait's turn after the cut does not un-cut the sitting. `TestAR83_2` stands as written.
2. **The erred results are counted.** `Tally.Errors`: the result events read with `is_error` true. Every `exit` line carries `errors` (an int, always written, 0 on the spawn-failure line), beside AR90's `results`. Turns, time, denials and the cost fold as AR83 rules.
3. **The stop stays.** A row whose last result erred, or that holds an erring cap, stays `failed` `stream-error`, and the night stops on it: §1 and D19, the owner's, settle that a sitting failure stops the night whatever its cause, and a child that ended on an API error ended without its work judged. Account stops are unchanged (`stream.go:210-221`).
4. **Act 1, the fold.** `TestAR91_1_LastResult` (stream; `Parse` as `TestAR83_2`, `parseFile` for fixtures): `error_during_execution` then `success`: not `IsError`, `success`, the success's tail, `Errors` 1, `Results` 2; `success` then `error_during_execution`: `IsError`, `error_during_execution`, `Errors` 1; `error_max_turns` then `success`: `IsError`, `error_max_turns`, `Errors` 1; two errors then a success: not `IsError`, `Errors` 2; `Errors` 0 for `ok.jsonl` and `two-results.jsonl`, 1 for `error.jsonl`; the fixture: not `IsError`, `success`, `NumTurns` 6, `DurationMS` 48500, `Denials` 2, `CostUSD` 1.25, `Errors` 1, `Results` 2, `Account` empty. The fixture, `recovered.jsonl`: `two-results.jsonl`'s 10 lines, its line 8 (the first result) with `subtype` `error_during_execution`, `is_error` true and `result` `API Error: 529 overloaded_error`, every other field and line as there.
5. **Act 2, the night.** `TestAR91_2_RecoveredGreen`: `RunRow` as `TestAR83_3` over `w.env("recovered.jsonl")`: the `exit` line's `is_error` false, `subtype` `success`, `results` 2, `errors` 1, `turns` 6, `cost_usd` 1.25; the gate green. `TestAR91_3_NightRunsOn`: `nightWorld` of r1 on the fixture, r2 on `ok.jsonl`, r3 on `error.jsonl`, r4 on `ok.jsonl`: `r1=green r2=green r3=failed r4=skipped`, the night `stopped:stream-error`, r3's `exit` line's `errors` 1. On the pin the night stops at r1.
6. **Act 3, the record.** relay-r1 §5: the `exit` keys gain `errors`; "One turn count": the end is the last result's, an erring cap kept, `errors` counts the erred (AR91). §11: the three tests. NICE's S3 gains `(AR91)`.
7. **Left, filed in act 3 (S4).** What the CLI exits with after an erring first result and a successful second is unmeasured, and the gate's exit fact also reads a non-zero exit (`run.go:878`), so a real recovered child may still read `stream-error`: a probe needs a forced API error. The result's `api_error_status` (`testdata/real.jsonl:9`) stays unread.

## 3. Interfaces (signatures, not prose)

`stream`: `Tally.Errors int`, the erring result events `Add` read; `fold` unexported as today, `IsError` and `Subtype` the later result's unless the kept one is an erring cap. `night`: the `exit` line's `errors` (an int, always written). No new event, flag, reason or function.

## 4. Findings that name this row

NICE "AR83's review (2026-10-14, the observer; two adversaries PASS, 0 findings)", its S3.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo (§1 quotes the survey).

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: each act's red line; rulings beyond §2; anything filed. Last line: branch, head sha.
