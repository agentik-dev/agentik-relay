# Packet AR65: the usage limit pauses the night

| item | value |
|---|---|
| from | `design/usage-limit-resume-r1.md` §5 packet 3, its first half (§2's pause and halt; §4 tests 1 to the halt, 2); `NICE_TO_HAVE.md`'s verify-round and AR64 notes; `design/relay-r1.md` §1, §5 |
| pin | agent-relay `main` at the merge of `land-ar60-ar61` (`df90422`: main plus AR60 to AR64); worktree `.claude/worktrees/ar65-limit-pause`, branch `ar65-limit-pause` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet ./internal/night ./internal/effects`, `go vet -tags e2e ./cmd/relay`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it; take a time's `.Location()`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/pause.go` (new), `internal/night/run.go`, `internal/night/night.go`, `internal/night/schedule.go` (`PickIn` only), `internal/effects/notify.go` (`Notices`' body only), `internal/night/ar65_internal_test.go` (new), `internal/night/ar65_test.go` (new), `internal/night/ar17d_test.go` (`usageText` only), `cmd/relay/e2e_test.go` (`TestE2E_10`'s text only), `design/relay-r1.md` (§1, §5, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^Test(AR65_\|AR17D_0_\|AR63_3_\|Pick\|NightEndToEnd)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR65_\|ok )"` reads 6; `go test ./cmd/relay -run "^TestAR59_4_Gofmt" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The record, §2, `pauseFor`: "It stops the night `usage-limit` at once: on a weekly limit; on a model's limit with no reset; when the wake is at or past `--until`; with no `--until`, when it is over 6 h away; [...] or while draining." "An unknown limit with no reset waits 30 minutes (source `fallback`)". "A night that ends paused halts each paused row `failed: usage-limit` (`Step(Paused, Halt)`), unlocks it, keeps its tree [...] and prints its `relay resume` line." A limited part's `sitting` line "gains `limit: paused` when the night pauses on it." The verify round: "Install packet 3 only with packet 4." `ar64-r1`: "packet 3 must discard a killed sibling's exit; night.go:1280 skips paused rows instead of failing them".

The code at `df90422`: `RunRow` kills on a refused event (`internal/night/run.go:344-346`), fails the part (`:515-516`) and writes the `sitting` line (`:523-524`); the row's goroutine unlocks its stage (`night.go:790`); `collect` kills every other running row (`:1008-1019`) and steps `OnSittingFailed` (`:1085-1086`); limits are read only while running (`:1185`); the poll is the `select` (`:1273-1282`); a row not terminal at the end is skipped (`:1292-1293`, the note's `:1280`); `PickIn` passes over busy and terminal rows only (`schedule.go:63`). `ar17d_test.go:54`, `:61` and `cmd/relay/e2e_test.go:808` use `usage limit reached|1758000000`, unknown and resetless to AR63.

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar65.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message.

1. **Act 1, `pauseFor`** (`pause.go`, pure). The wake: with no reset (an unknown or window kind), `Now` plus 30 min, source `fallback`; else the reset plus 2 min, the Limit's source. A stop, its `Detail` naming why: draining; `weekly`; a model's with no reset; the wake at or past `Until`; no `Until` and the wake over 6 h after `Now`. Else a pause, a model's with a reset too.
2. **Act 1, `PickIn`** never picks a paused row; its dependents wait, never doomed.
3. **Act 2, the held line.** The night sets `RunOptions.hold` on every row (nil: as today). A part ending failed `usage-limit` (before the spawn too) writes no `sitting` line: `RunRow` fills `hold` with `tally.Limit(hit)` (zero for a row another's stop killed) and the writer, and the goroutine leaves its stage locked. `collect` decides, writes it first (`limit: paused` when the night pauses on the part), then acts; it unlocks the stage unless paused. `Notices` refuses a `sitting` line whose `limit` is `paused` (relay-r1 §1: the `pause` line's notice serves).
4. **Act 2, the pause.** In `collect`, a row already paused (a killed sibling's late exit) is discarded: its held line, if any, written paused (none: its slot freed), no night step. Else a part whose Limit has a kind, the night not stopped: `pauseFor` (`Now` the clock, `.Round(0)`). A stop is today's path, the `stop` line's `detail` its Detail. A pause: the part `Step(Exited, Limited)`, reason `usage-limit`, `Staged` kept; every other running row killed as today and set paused at once; `OnLimited`, the count up; a `pause` line (`rows` sorted, `kind`, `resets_at`, `source`, `wake` RFC3339, `count`). A model's limit pauses every row until packet 5.
5. **Act 3, the poll.** While paused: no pick, no `checkWindows` (N1); each poll reads `fx.Clock.Now().Round(0)`, no timer. At or past the wake: `until` passed, `OnUntil`; `--max-usd` reached, `OnBudget`; else `OnSittingFailed` `usage-limit`, detail `woke at <wake>` (packet 4's wake goes there).
6. **Act 3, the halt.** At the end, before the skip, each paused row: `Step(Paused, Halt)`, reason `usage-limit`, unlocked, tree kept, `relay resume <id>` printed and in the `stop` line's `resume`. No second `sitting` line and no notice (N7).
7. **The tests the pause breaks**, in act 2: `usageText` and `TestE2E_10`'s result become `You've hit your weekly limit · resets Sep 28 at 4pm (America/Los_Angeles)`, which still stops.
8. **Tests.** internal: `TestAR65_1_PauseFor` (each stop and Detail; the fallback; a model with a reset; the wake a minute before `Until`, at it; with none, a wake 6 h on, 6 h 1 min on). `TestAR65_2_PickSkipsPaused`. Nights on `effects.FakeSpawner` (a `rejected` `five_hour` `rate_limit_event`, `resetsAt` T+1h; exit 143) and `effects.StepClock`, every wait bounded: `TestAR65_3_PauseHolds` (r1 to r3, `--parallel 2`, r2's child a stall its kill ends (a zero Limit): one `pause` line, rows `[r1 r2]`, `window`, `event`, wake T+1h+2m, count 1; status `paused:usage-limit`, both rows `paused`; each `sitting` line `failed`, `usage-limit`, `limit: paused`, no `sitting` notice; ten still polls add no line, no spawn; SIGINT on `Signals`: `owner-kill`); `TestAR65_4_StopsAtOnce` (a `seven_day` event, the Fable text, a reset past `--until`, one 7 h on: `stopped:usage-limit`, no `pause` line, the `stop` line's detail the Detail; `API Error: 401`: `stopped:auth`); `TestAR65_5_WakeHalts` (T+1h+1m paused; T+1h+2m: `stopped:usage-limit`, r1 failed `usage-limit`, `night.Locked` false, its tree there, one `sitting` line, in the `stop` line's `resume`; `--max-usd` 1: `budget-reached`).
9. **The record, in act 3.** relay-r1 §1: `usage-limit` names `pauseFor`'s stops and the pause (AR17D act 0 now weekly); the pairs the loop steps replace "steps none"; §5: the `pause` line, the `sitting` line's `limit`; §11: the tests. The verify-round and `ar64-r1` notes gain `(AR65)`.
10. **Not done.** The owner while paused, `redSittings`, `clean --merged`, the notice (AR66); the wake (packet 4); the count, a later or the same reset, the probe, a model's scope, a sibling gated again (packet 5); the readers (packet 6); `Night.tla`. AR65 installs only with AR66 and packet 4.

## 3. Interfaces (signatures, not prose)

`night`: `type pauseIn struct { Limit stream.Limit; Now, Until time.Time; Draining bool }`, `type pause struct { Stop bool; Detail, Source string; Wake time.Time }`, `func pauseFor(in pauseIn) pause`, `type heldLine struct { Limit stream.Limit; write func(paused bool) }`, `RunOptions.hold *heldLine`. Tests `func TestAR65_<n>_<Name>(t *testing.T)`; red on the pin: `_1` (undefined), `_2` (the paused row picked), `_3` (no `pause` line), `_4` (an empty detail), `_5` (no halt at the wake).

## 4. Findings that name this row

The record's §1: five ledger hits, each a night stopped `usage-limit`. The owner (observer, 2026-10-10): "sessions and relay runs resume by themselves after a usage limit (the 5-hour window), instead of stopping."

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line with its count, `go vet`'s exits, `make doclint`'s exit and `make hygiene`'s last line. Bullets: the TestAR65 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
