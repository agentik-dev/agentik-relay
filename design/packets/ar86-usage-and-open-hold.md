# Packet AR86: the exit line's output from the result, and the report's open band hold

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`: "The observer's check-in 123 (2026-10-15; S4)" and "The observer's reading of us4b's exit line (2026-10-15 05:40; S4)" |
| pin | `main` at 87776d9 plus this packet's own commit; a linked worktree, branch `ar86-usage-and-open-hold` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` or `agentik relay` verb against the account's state: a night may be live. Never run `agentik-session`. |
| may touch | `internal/stream/stream.go` (`wire`'s top-level usage, `Decode`, `fold`, `Tally` and its doc comment), `internal/stream/ar86_test.go` (new), `internal/night/report.go` (`BandHolds`, `BuildReport`'s band holds, `Markdown`'s band line), `internal/night/ar86_test.go` (new), `NICE_TO_HAVE.md` (the two sources' entries: mark them landed as AR86) |
| acceptance | `go test ./internal/stream -run '^TestAR86_' -count=1 -timeout 120s -v 2>&1 \| grep -c -E '^--- PASS: TestAR86_'` reads 2; `go test ./internal/night -run '^TestAR86_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR86_'` reads 2; `go test ./internal/stream -run 'TestAR28\|TestAR83\|TestParseFixtures\|TestTally\|TestBuildersCounted\|TestTurnDedup' -count=1 -timeout 120s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0; `go test ./internal/night -run 'TestAR81\|TestAR83\|TestAR28\|TestAR55' -count=1 -timeout 600s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0 |
| budget | 40 turns, 140k context, `claude-opus-5-5`; two acts, one commit each, and one for NICE |
| verify | `make check` |

## 1. What the source says (quoted)

NICE, us4b: "The exit and turn lines' `usage.output` under-counts by two orders: night 2026-10-15-2's us4b exit line reads `"output":675` for 70 turns and $5.85, while its transcript holds 103,326 output tokens over 65 main-thread message ids. `stream.Tally` keeps each message id's last `usage` from the stream's `assistant` events (`internal/stream/stream.go:207-215`), and those carry the message's opening count, not its final one. A fix: take `output` from the `result` event's `usage` when it lands."

NICE, check-in 123: "`report` counts an open band hold as no wait: night 2026-10-15-1 read `band: 1 hold, 0s waited` at 02:33 for po2c held since 01:30:41 (62 asks), since `wait_s` lands only on the hold's end line (AR81). Count an open hold to now, as `status --json`'s `band.since` already allows, or say `held since HH:MM`."

## 2. Rulings

1. **The result's usage (act 1).** `Decode` lifts a `result` line's top-level `usage` (`input_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, `output_tokens`) into `Event.Usage`; `internal/stream/testdata/real.jsonl`'s result line carries one (`output_tokens` 41 where its assistant events read 4). `fold` sums two results' usage, as it sums `num_turns` (AR83: per query).
2. **The tally (act 1).** `Tally` keeps the builders' streamed output (messages with a `parent_tool_use_id`) apart from the main thread's. When a result with a usage is read, `Tally.Usage.Output` is the folded result's output plus the builders' streamed output; `Input`, `CacheRead` and `CacheCreate` stay the stream's, and `Context` is untouched. With no result, or a result with no usage, `Usage` is as on the pin. The `turn` lines, written before any result, stay as they are. `Tally`'s doc comment says this in two sentences.
3. **The open hold (act 2).** `BandHolds` gains `Open int` (`json:"open,omitempty"`) and `OpenSince string` (`json:"open_since,omitempty"`): after `BuildReport`'s lines loop, each hold still in `holding` counts in `Open`, and `OpenSince` is the earliest such held line's `ts` as the ledger wrote it. `WaitS` does not change (the report has no clock). A night with no open hold reads the same JSON and markdown as on the pin.
4. **The line (act 2).** With `Open > 0`, `Markdown`'s band line reads `band: 1 hold, 0s waited, 1 open since 2026-10-15T08:30:41Z` (the open count through `plural`'s shape is not needed: `N open since <ts>`).
5. **Tests.** `TestAR86_1_ResultOutput` (stream): `Parse` over `testdata/real.jsonl` reads `Usage.Output` 41; red on the pin. `TestAR86_2_TwoResultsAndBuilders` (stream): an inline stream of a main message (output 5), a builder message (output 10), and two results (output 100, then 30) reads `Usage.Output` 140; the same stream with no result reads 15. `TestAR86_3_ReportOpenHold` (night): ledger lines with a `night-open`, a `band` `held` line for one row and no end line read `Band{Holds:1, WaitS:0, Open:1, OpenSince:<that ts>}` and the markdown line above; red on the pin. `TestAR86_4_ClosedHoldUnchanged` (night): a held line then an `admitted` line with `wait_s` 120 reads `Open` 0, `OpenSince` empty, and `band: 1 hold, 2m0s waited` with no `open`. Use AR81's report fixtures' shape (`TestAR81_2_ReportHolds`) rather than a new world.

## 3. Interfaces (signatures, not prose)

`night.BandHolds{Holds, WaitS int; Open int; OpenSince string}`. No new exported name in `stream`; no new ledger event or key: the exit line's `usage` keeps its four keys.

## 4. Findings that name this row

The two NICE entries above. The builders' streamed output stays an opening count (status's `builder output` reads the builders' transcripts and is unaffected); name it in the NICE entry as what AR86 leaves.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory and the session plugin's. Read no other repo.

## 6. Report (capped)

At most 120 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits. Bullets: the red lines of `TestAR86_1` and `TestAR86_3` on the pin; any ruling beyond §2.
