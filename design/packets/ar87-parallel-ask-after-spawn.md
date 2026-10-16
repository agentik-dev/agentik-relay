# Packet AR87: a parallel night's next band ask waits until the admitted row shows to the band

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`: "The refuter's F3 (2026-10-15)"; `design/band-pacing-r1.md` §2.4; the observer's rulings (2026-10-15) |
| pin | `main` at f0de0fa plus this packet's own commit; a linked worktree, branch `ar87-parallel-ask-after-spawn` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` or `agentik relay` verb against the account's state: a night may be live. Never run `agentik-session`. Hygiene's `lan` rule matches the Go name of the local zone: never spell it. |
| may touch | `internal/night/run.go` (`BandSettle`, `bandQueue`, `askBand`'s leaves and its one log sentence, `RunRow`'s `shown` calls), `internal/night/ar87_test.go` (new), `internal/night/ar80_test.go` (`ar80Spawner` alone), `internal/night/ar80_fix1_test.go` (`TestAR80_11`'s child alone), `internal/night/ar81_test.go` (`TestAR81_1`'s child alone, only if it needs it), `design/band-pacing-r1.md` (§2.4's queue sentence), `NICE_TO_HAVE.md` (the F3 entry) |
| acceptance | `go test ./internal/night -run '^TestAR87_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR87_'` reads 5; `go test ./internal/night -run 'TestAR28\|TestAR65\|TestAR67\|TestAR69\|TestAR71\|TestAR80\|TestAR81\|TestAR85' -count=1 -timeout 600s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0 |
| budget | 60 turns, 160k context, `claude-opus-5-5`; one act, one commit, and one for NICE |
| verify | `make check` |

## 1. What the source says (quoted)

NICE, the refuter's F3: "In a parallel night AR80 lets row B ask the band the moment row A is admitted, before A spawns (`bandQueue`'s leave, `internal/night/run.go`'s `askBand`), so B's ask never sees A as a running thread. … A fix: hold the next row's ask until the admitted row has spawned (or its first reading lands)."

band-pacing-r1 §2.4: "Held rows queue; only the first asks, the next one poll after it spawns." AR80 ruling 6 built "the next asks at once after an unheld head": `leave` (`run.go:225`) zeroes `q.after` (`:233`); the night builds the queue at `night.go:787-789`.

agentik-session (read for this packet): `project` (`claude_session/admission.py:708`) counts a `relay-row` in a·T "only once it is active (a ledger reading in ACTIVE and no end line since), and until then adds a·min(T5, L_row)" for the asking row alone (`:730-731`); `_active` (`:570`) wants a `reading` line within 45 minutes (`:386`). The plugin's hook writes that line on `session.measure` when `changed` holds `rateLimits` (`plugin/hooks/session.ts:330-337`), its record bounded at 7 s (`LIMIT_MS + 2 * BOUND_MS`), and `session.measure` fires "after each main-thread turn", its `rateLimits` "empty … before the first reading" (`design/admission-r1.md:76`). The session's `start` line carries no reading.

Measured over the 12 rows that asked (nights 2026-10-14-6 to 2026-10-15-5), the plugin's ledger against each row's main-thread message times: the first reading trailed the row's first message in 11 of 12, by up to 5.0 s (po1d), and led its second message in 12 of 12, by 2.7 s or more (the ledger keeps whole seconds). The second message came 8.8 to 32.1 s after the `spawn` line.

## 2. Rulings

1. **The signal.** A row has shown to the band at its second main-thread message: the `assistant` event with no `parent_tool_use_id` at which `Tally.Turns` first reads 2, in `RunRow`'s stream callback (`run.go:705`). The first message is not enough (§1: the reading trails it); the first `turn` line waits for turn 10 or a zone crossing (`run.go:709`).
2. **The wait.** When the queue's head leaves on a spawning verdict (`admitted`, `unread`, `stale` or `unchecked`), it settles: the next head asks no sooner than the first of that row showing, its child's exit (`proc.Wait` returns, `run.go:738`), `RunRow` returning before a spawn (breaker, stop, kill, a failed `spawn` line or start), and `BandSettle` after the leave. A head held before its spawning verdict keeps its BandPoll mark as well: the next asks at the later of the two. A `reset` end, a cut and `--until` settle nothing (the rows behind a reset hold to the same reset; AR80 ruling 6). Only the head's leave settles, so one row settles at a time.
3. **The cap.** `BandSettle` is 2 minutes, through `fx.Clock`: the second message came at most 32.1 s after the spawn and the spacing adds at most 30 s (`SpawnSpacing`), so it is about twice the worst seen, and a row that never shows costs the next ask two polls at most. Since the next spawn already waits `SpawnSpacing`, the usual cost is none.
4. **`--parallel 1`** is as on the pin: a row's `RunRow` returns, ending its settle, before the next row launches, so the next ask's time, the `band`, `spawn` and `sitting` lines, and the BandPoll mark after a held head do not change.
5. **The trace.** No ledger line, key, status field or golden changes; `band` lines keep their shape. The waiting head's row log gets one sentence per settle it waits on: `relay <row>: the band waits for <settling> to show`.
6. **The record.** band-pacing-r1 §2.4's queue sentence reads: "only the first asks; the next asks once the row ahead has shown to the band (its second main-thread message), exited, or waited `BandSettle`, and no sooner than one poll after a held one (AR87)".
7. **Old tests.** `TestAR80_10` and `TestAR80_11`'s two spawned cases let r2 ask while r1 is spawned and silent: the defect. Change only r1's child so it shows first (`ar80Spawner`'s `wait` and `stall` children write two main-thread messages before their silence); every assertion stays, the still clock included. `TestAR81_1` changes only if it fails.
8. **Tests**, in `internal/night/ar87_test.go`: `ar80World` on a `FakeBand` and the step clock, T `ar65T`, `--parallel 2` unless named; r1's child an `io.Pipe` the test writes main-thread `assistant` lines into (ids `m1`, `m2`) and closes for a clean result and exit 0.
   - `TestAR87_1_NextWaitsForShown`: r1 admitted at T, its child writes `m1` and waits: 300 ms after `m1` is read the asks read `r1 +0s` (red on the pin: `r1 +0s, r2 +0s`); then `m2`: r2 asks at +0s, the clock still, after `m2` is read; r2's row log holds ruling 5's sentence once.
   - `TestAR87_2_CapEndsWait`: r1 admitted, its child silent until its kill: no r2 ask at +119 s; at +120 s r2 asks (`r1 +0s, r2 +120s`); `kill r1` ends the night.
   - `TestAR87_3_ExitEndsWait`: on the still clock, r1's child writes `m1` and ends clean; r1's child is `ar67Clean` (no message); r1's start errs. Each reads `r1 +0s, r2 +0s` and the night ends with the clock at T.
   - `TestAR87_4_HeldHeadKeepsPoll`: r1 `not-started`, then admitted at +60 s. A child writing `m1`, `m2` at once reads `r1 +0s, r1 +60s, r2 +120s` (as `TestAR80_5`); one writing `m1` and waiting reads no r2 ask at +120 s, then r2 at +180 s (+60 s plus `BandSettle`).
   - `TestAR87_5_ParallelOneAsBefore`: `--parallel 1`: r1 admitted with `ar67Clean` reads `r1 +0s, r2 +0s`; r1 `not-started` then admitted at +60 s reads `r1 +0s, r1 +60s, r2 +120s`; r2's sitting `band` reads `map[asks:1 verdict:admitted wait_s:0]` and no row log holds the sentence.

## 3. Interfaces (signatures, not prose)

`night`: `var BandSettle = 2 * time.Minute`. Unexported: `bandQueue` gains `settling string` and `settleBy time.Time`; `func (q *bandQueue) leave(row string, held, spawns bool)`; `func (q *bandQueue) shown(row string)` (a nil queue, or a row not settling: nothing); `place(row)` adds `waits string`, the settling row, and its `after` is the later of the BandPoll mark and `settleBy` while a row settles. No new exported name in `effects`; no ledger event, key or golden change.

## 4. Findings that name this row

NICE's F3 entry (its replay of agentik-session's fix is that repo's). Mark it landed as AR87 and name what AR87 leaves: the signal is measured, not proven (a CLI that starts the second message before the hook's record returns would let a reading trail it, by the record's 7 s at most); a row whose first turn is a long tool call holds the next ask to `BandSettle`.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory and the session plugin's. Read no other repo; §1 quotes agentik-session.

## 6. Report (capped)

At most 120 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits. Bullets: `TestAR87_1`'s red line on the pin; each existing test changed, and how; any ruling beyond §2.
