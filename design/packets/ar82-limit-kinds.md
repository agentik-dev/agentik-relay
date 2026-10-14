# Packet AR82: the usage-limit kinds a refused event names

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`'s research item 6 ("`rate_limit_event`'s spelling and the per-family weekly values", 1d14d3a) and AR63's note on a `rateLimitType` outside the map; `design/usage-limit-resume-r1.md` §2 (the kind, the pause) and §6 (the stream's fields); `DECISIONS.md` entry 12; the Agent SDK Python reference's `RateLimitInfo` and the TypeScript reference's `SDKRateLimitEvent` (code.claude.com, as of 2026-10-14); `internal/stream/limit.go` and `internal/night/pause.go` at 1d14d3a |
| pin | agent-relay `main` at 1d14d3a plus this packet's own commit; a linked worktree, branch `ar82-limit-kinds` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/stream ./internal/night`, `gofmt -l internal/stream internal/night`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/stream/limit.go` (the kinds, `eventKinds`, `Tally.Limit` and their comments only), `internal/stream/ar82_test.go` (new), `internal/night/pause.go` (`pauseFor` and its comment only), `internal/night/ar82_internal_test.go` (new), `internal/night/ar82_test.go` (new), `design/relay-r1.md` (§1's `pauseFor` sentence, §5's `limit` kinds, §10's kind sentence, §11's test list only), `design/usage-limit-resume-r1.md` (§6's "The stream's fields" bullet only), `NICE_TO_HAVE.md` (item 6 and AR63's note, a mark each; a new section for what the build leaves), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/stream -run '^TestAR82_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR82_'` reads 1; `go test ./internal/night -run '^TestAR82_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR82_'` reads 3; `go test ./internal/stream -run '^Test(AR17D_\|AR20_6_\|AR63\|AR69_\|AR82_)' -count=1 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./internal/night -run '^Test(AR63_\|AR65_\|AR69_\|AR71_\|AR82_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 |
| budget | 60 turns, 150k context, `claude-opus-5-5`; two acts, one commit each |
| verify | `make check` |

## 1. What the source says (quoted)

NICE item 6 (the research fixes): "`eventKinds` maps only `five_hour` and `seven_day`. … give `seven_day_opus` and `seven_day_sonnet` the model-limit kind (only that family's rows pause), `overage` a kind or a stop, with tests." AR63's note: "a `rateLimitType` other than `five_hour` or `seven_day` (`seven_day_opus`) falls to the text's kind." The record, §2: "The kind is the event's `rateLimitType`, else the text's"; "a model's limit pauses only that model's rows"; `pauseFor` stops at once "on a weekly limit; on a model's limit with no reset". Entry 12: "When one model's own limit is hit and names its reset, only that model's rows pause; rows on other models keep running".

The references: Python's `RateLimitType = Literal["five_hour", "seven_day", "seven_day_opus", "seven_day_sonnet", "overage"]`; `rate_limit_type` "Which rate limit window applies"; `overage_status` "Status of pay-as-you-go overage usage, if applicable"; `raw` the CLI's dict. TypeScript's `rate_limit_info` spells `resetsAt` and carries `errorCode?: "credits_required"` (Claude Code 2.1.181 or later).

The code at 1d14d3a: `eventKinds` (`internal/stream/limit.go:71`) holds two types; `Tally.Limit` (`:95-97`) takes any other to `textKind`, which reads an empty or unmatched text as unknown. So a refused `seven_day_opus` or `overage` event with a usable `resetsAt` pauses every row (`pauseFor`, `internal/night/pause.go:85-117`, scope "").

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar82.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is.

1. **Act 1, the table.** `eventKinds` maps each documented type to a kind and a scope: `five_hour` the window, `seven_day` weekly, `seven_day_opus` a model's (scope `opus`), `seven_day_sonnet` a model's (scope `sonnet`), `overage` the new kind `overage`. A type outside the table, or none, falls to the text's kind as today; `resetsAt` gives the reset whatever the kind. Exact values only: an undocumented `seven_day_<word>` is outside.
2. **A family's weekly cap is a model's limit** (entry 12). `pauseFor` does not change for it: with a reset it pauses only the rows `ofModel` holds (`claude-opus-5-5` holds `opus`, `claude-sonnet-4-5` `sonnet`); with none it stops (`the opus limit names no reset`); a wake at or past `--until`, or over 6 h with no `--until`, stops the night as for any model's limit.
3. **`overage` stops the night at once** (the record names no overage: the most conservative reading, ruled here). The references name no model for overage ("pay-as-you-go overage usage"), so the pool is the account's. Scoping it to the hit row's model, the other reading, would send the other models' rows into the same refusal; waiting for its reset would hold the night on a cap only the owner can raise. `pauseFor` stops on it after the weekly case, detail `the overage limit`, its wake and source kept as the weekly stop keeps them. Its own kind keeps the `exit` line true (`kind: overage`), so the morning read does not wait for a weekly reset.
4. **The wire spelling stays** `rateLimitType` and `resetsAt` under `rate_limit_info`: the TypeScript reference spells `resetsAt` as `testdata/real.jsonl:8` does, and the Python snake case is its dataclass over the CLI's `raw` dict. No snake-case reading; the re-test on a real child stays filed.
5. **Tests.** `TestAR82_1_EventKinds` (stream): refused events through `Parse`, `resetsAt` the hit plus 1 h: `seven_day_opus` reads `model`, scope `opus`, that reset, `event`; `seven_day_sonnet` `model`, `sonnet`; `overage` `overage`, scope ""; `five_hour` and `seven_day` as before; `seven_day_opus` with no `resetsAt` `model`, `opus`, no reset; an undocumented `seven_day_haiku` before the session text, the window from the text; each `Line()`'s `kind` and `scope`. `TestAR82_2_PauseForKinds` (night, internal): `overage` with a reset 1 h on and no `--until` stops, `the overage limit`, `event`, the reset plus 2 min; `model` scope `opus` with that reset pauses, scope `opus`; `ofModel`: `claude-sonnet-4-5` holds `sonnet`, `claude-opus-5-5` does not. On TestAR71's world with r1 on `claude-opus-5-5`, r2 and r3 on `claude-fable-5-1`: `TestAR82_3_FamilyWeekly`, r1's part 1 a refused `seven_day_opus` event, `resetsAt` 02:00, exit 143: one `pause` line `[r1]`, `model`, scope `opus`, wake 02:02, no `kill` line; r2 and r3 green before the clock moves; at 02:02 one `wake` line `[r1]`, r1's part 2, all green, `queue-empty`, 4 spawns. `TestAR82_4_OverageStops`, TestAR65's world, r1's child a refused `overage` event resetting 1 h on: `stopped:usage-limit` at once, `r1=failed r2=skipped`, no `pause` line, the `stop` line's detail `the overage limit`, r1's `exit` line's `limit.kind` `overage`. Tests spell kinds as the ledger does, so each compiles and reads red on the pin.
6. **Act 2, the record.** relay-r1 §1: `pauseFor`'s stops gain the overage limit; §5: the `limit` `kind` list gains `overage`; §10: the event's five types; §11: the four tests. usage-limit-resume-r1 §6's bullet: the kinds mapped (AR82), the re-test still filed. NICE item 6 and AR63's note gain `(AR82)`.
7. **Left, filed in act 2.** A family's weekly reset lies days on, so under ruling 2 it mostly stops the whole night (§2's stops, §6's first risk); holding only its rows to the night's end while the others run changes the record: a NICE note, S3. The TypeScript `errorCode` `credits_required` is not read: a NICE note, S4.

## 3. Interfaces (signatures, not prose)

`stream`: `const KindOverage = "overage"`; `eventKinds` a type to its kind and scope. `night`: `pauseFor`'s case `l.Kind == stream.KindOverage`, detail `the overage limit`. No new line, key or event; the `exit` line's `limit.kind` gains a value, never a `pause` line's.

## 4. Findings that name this row

NICE research item 6 (S3, 1d14d3a) and AR63's note on a type outside the map; usage-limit-resume-r1 §6's "The stream's fields" bullet.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo; section 1 quotes the references.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: each TestAR82 red line on the pin; the rulings beyond §2; anything filed. Last line: branch, head sha.
