# Packet AR92: a family's cap past the night's end shelves that family's rows; the others run on

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`, "AR82's build (2026-10-14): what it leaves", its S3; `DECISIONS.md` entries 12 and 11 (§1); `design/usage-limit-resume-r1.md` §2, §6; `design/relay-r1.md` §1, §5; the survey `research/synthesis/tools-relay.md` in agentik-research, §2.9; the code at 8505f11 |
| pin | `main` at a018012 (AR91 merged) plus this packet's own commit; a linked worktree, branch `ar92-family-shelve` |
| wall | AR83's: this repo only; no push, `--amend`, rebase, device, ssh or network; no `relay` binary by hand or outside `make`'s harness; no `relay` or `agentik relay` verb on the account's state; never `claude`. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/night`, `gofmt -l internal/night`, `make doclint`, `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Never spell the Go name of the local zone. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/pause.go` (`pause.Shelve`, `pauseFor`, their comments), `internal/night/night.go` (the shelve at the sites §1's code line names, with their comments only), `internal/night/status.go` (`RowStatus.Shelved`, `buildStatus`, `PrintStatus`), `internal/night/report.go` (`StoppedRow.Shelved`, `BuildReport`, `Markdown`), `internal/night/ar92_internal_test.go` (new), `internal/night/ar92_test.go` (new), `design/usage-limit-resume-r1.md` (§2's pause paragraphs, §6's first risk only), `design/relay-r1.md` (§1's night states, §5's line keys, §11's test list only), `NICE_TO_HAVE.md` (the S3 item, a mark; one new section), `history/sittings/*.md`, `history/sessions/NEXT`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR92_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR92_'` reads 8 (fix round 2); `go test ./internal/night -run '^Test(AR65_\|AR67_\|AR69_\|AR70_\|AR71_\|AR77_\|AR82_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./internal/night -run '^Test(FailedStops\|BlockedStops\|StatusResumeLines\|Report)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 |
| budget | 90 turns, 180k context, `claude-opus-5-5`; three acts, one commit each; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says (quoted)

NICE (S3): "a model family's weekly cap (`seven_day_opus`, `seven_day_sonnet`) resets days on, so `pauseFor` stops the whole night on it whenever the wake is at or past `--until` or over 6 h away with none, … : the rows on the other models are killed with it. Holding only that family's rows to the night's end while the others run on would change the record; a design question, with entry 11's `limit_fallback_model` beside it."

`DECISIONS.md`, verbatim. Entry 12: "When one model's own limit is hit and names its reset, only that model's rows pause; rows on other models keep running, and the paused rows continue at the reset (packet AR71). The five-hour window's limit still pauses the whole night, as the owner ruled on 10-10. Portfolio entry 23's amendment gains this sentence." Entry 11: "A repo's relay config may name `limit_fallback_model`; when it is unset, nothing changes. When it is set, a row stopped by its own model's limit continues at once on that model, in its own tree (packet 7). Only a less critical repo sets it; example-app never does."

The record. usage-limit-resume-r1 §2 (2026-10-10, before entry 12): `pauseFor` stops the night "when the wake is at or past `--until`; with no `--until`, when it is over 6 h away"; "A night that ends paused halts each paused row `failed: usage-limit`". Its proposed "a reset past the run's end … stops the whole run, not one row" is not in portfolio entry 23. AR82 kept the stops for the family caps "as for any model's limit". The survey (§2.9): "a model limit with a reset pauses only that model's rows".

The code at 8505f11: `pauseFor`'s late-wake stops (`internal/night/pause.go:111-114`); in `night.go` the hit (`:1399-1436`), a killed sibling's (`:1314`), `holdRunning` (`:1260`), `wakeUp` (`:1614`), `pickPaused` (`:1688`), the running pick (`:1774`), its blocked and queue-empty steps (`:1832-1834`), the halt (`:1864`), the `stop` line (`:1907`); `eventKinds` (`internal/stream/limit.go:77-83`).

## 2. Rulings

Standing rules. The charter, `history/sittings/<date>-<n>-ar92.md`, opens at act 1 and closes before the last commit; no home path. Each act's first test is red on the pin, its red line in the commit message. `DECISIONS.md` stays as it is: entry 12's letter rules this row; §2's older stops are amended here.

1. **A late model wake shelves.** `pauseFor`: a model's limit with a reset (a family's weekly cap too) whose wake is at or past `--until`, or with no `--until` over 6 h away, no longer stops: `Shelve` true, `Detail` the stop's text as today, `Scope` the limit's, `Wake` the reset plus 2 minutes. Every earlier stop (a model's with no reset, the guards, 12 pauses among them) and every other kind stay.
2. **A shelved family waits out the night.** The scope's running rows are killed and paused as AR71 does it, a killed sibling landing shelved; its rows are never picked (both picks hold them), continued or woken tonight, even once the reset passes. A shelve counts one pause, arms no wake and leaves the night running: the others run on under its rules, `--until`, `--max-sittings` and `--max-usd` included.
3. **The night's end.** When nothing outside the shelved scope runs or can be picked (queue-empty, or blocked behind a shelved row), the night stops `usage-limit`, the shelve's detail; any other end keeps its reason. At every end a shelved row halts as a paused row does: `failed` `usage-limit`, stage unlocked, tree kept, resume line printed; an unstarted row of the scope is `skipped`. The `stop` line gains `shelved`, the shelved rows.
4. **A later hit** is decided as today, the shelved rows aside: a stop stops the night; a pause pauses the rows outside the shelve, and its own wake continues only them.
5. **Status and the report.** A shelve's `pause` line gains `shelved: true`. `RowStatus.Shelved` is the wake (RFC 3339): `status` shows a shelved row `paused` `usage-limit`, shelved to the night's end; its `next` names the `usage-limit` end while a shelve stands. The report's stopped row says shelved. Entry 11's fallback (packet 7) is unbuilt; once set, it continues a row before any shelve.
6. **Act 1, `pauseFor`.** `TestAR92_1_ShelveFor` (internal): scope `opus`, a reset 72 h on, `--until` 4 h on: not `Stop`, `Shelve`, the `--until` detail, wake 72 h 2 min on; no `--until`: `Shelve`, the 6 h detail; a reset 1 h on: a plain pause; `Count` 12: a stop; a window past `--until`, a weekly, a model's with no reset: stops.
7. **Act 2, the night.** `TestAR92_2_FamilyShelved`: `TestAR82_3`'s world cut to two rows, `--parallel 2`, `--until` `ar65T` plus 4 h; r1 on `claude-opus-5-5`, its part 1 a refused `seven_day_opus` event resetting 72 h on, exit 143; r2 on `claude-fable-5-1`, silent until the test releases it. One `pause` line `[r1]`, scope opus, `shelved` true; no `kill` line; before the release `ReadStatus` reads the night `running`, r1 `paused` `usage-limit` with its `Shelved`, r2 running; then, the clock unmoved: `r1=failed r2=green`, `stopped:usage-limit`, the `stop` line's `shelved` `[r1]` and the `--until` detail, r1's resume line, 2 spawns, the report's r1 shelved. On the pin r2 is killed at the hit. `TestAR92_3_WindowUnchanged`: the same world, r2 released at the `pause` line, r1's event `five_hour` resetting 1 h on: one `pause` line `[r1 r2]`, scope "", no `shelved`, r2's `kill` line, the night paused; at 02:02 both continue, all green, queue-empty.
8. **Act 3, the record.** usage-limit-resume-r1 §2's pause paragraphs and §6's first risk: the shelve (AR92). relay-r1 §1: the shelve beside `paused:usage-limit`; §5: the two lines' `shelved`; §11: the three tests. NICE's S3 gains `(AR92)`.
9. **Left, filed in act 3 (S4).** A model's limit with no reset still stops the night (entry 12 names a reset); `Night.tla` models no shelve.

## 3. Interfaces (signatures, not prose)

`night`: `pause.Shelve bool`; `RowStatus.Shelved string` (json `shelved`, "" when not); `StoppedRow.Shelved bool` (json `shelved,omitempty`); the `pause` line's `shelved` (true, a shelve's only); the `stop` line's `shelved` (row ids, always written). No new state, stop reason, event, flag or exported function.

## 4. Findings that name this row

NICE "AR82's build (2026-10-14): what it leaves", its S3; usage-limit-resume-r1 §6's first risk.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo (§1 quotes the survey).

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: each act's red line; rulings beyond §2; anything filed. Last line: branch, head sha.
