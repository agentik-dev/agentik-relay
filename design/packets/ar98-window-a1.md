# Packet AR98, Amendment 1: round 1's findings

| item | value |
|---|---|
| from | the refuter's round 1 on 0b495d6, FAIL on one block and three S4s; the coordinator's rulings below; packet `design/packets/ar98-window.md`, whose wall and forbidden reads stand |
| pin | `ar98-window` at 0b495d6; worktree `.claude/worktrees/ar98-window` |
| wall | the parent packet's, unchanged |
| may touch | `internal/night/run.go`, `internal/night/ar98_test.go`, `internal/effects/band.go`, `internal/effects/ar98_test.go`, `design/packets/ar98-window-a1.md`, the sitting's charter, `NICE_TO_HAVE.md` |
| acceptance | the parent packet's acceptance lines, each reading as it does there, but the night's `^TestAR98_` count reads the new total, named in the report; `git diff --stat 57aba66 -- internal/night/ar80_test.go internal/night/ar65_test.go internal/night/ar69_test.go` prints nothing; `make doclint` exits 0; `make hygiene` reads `hits outside allowlist: 0` |
| budget | 40 tool calls, 180k context, `claude-opus-5-5`; acts: 0 this file; 1 the tests, red; 2 the code; one commit per act |
| verify | `make check`, by the coordinator |

## 1. What the source says

The refuter's round 1 on 0b495d6: FAIL on F1, with F2, F3 and F4 as S4s; the coordinator's rulings are §2.

## 2. Rulings

1. **F1, block.** A `wait` with an empty `Window` and `Reset()` erring goes `unchecked` and spawns, as on main: the parent's ruling 1 wins over ruling 4's literal text, which is read as limited to an answer that names a window. `run.go`'s no-reset hold gains the guard. Test: a fake answering `wait` with no `window`, `Reset` erring, reads `unchecked`, spawned at once.
2. **F2 and F3, taken, since a row must not spawn on a spent week while its head holds.** The queue carries the head's reset: when a head opens a hold, `q.hold` records its reset, zero or not. A row queued behind a held head takes that reset rather than asking `Reset()`; so it neither goes `unchecked` on `Reset()`'s error nor overtakes a head held to the week's reset. A head's later hold moves it for a row that opens after. A row queued behind a head that holds with a zero reset holds bare, as today. `TestAR80_7` and every pause test stay green unchanged; if one cannot, stop and report, changing nothing in it. Tests: `TestAR98_6_QueuedTakesHeadReset`, r1 `denied` with `resume_at` a week out and `Reset` erring, r2 held `queued`, spawned never within the week; and the same with `Reset` answering a five-hour reset, r2 held past it.
3. **The long hold stays.** A `five_hour` `denied` holding to the week's reset with no `--until` is what the record rules; no change.
4. **F4, taken.** `ReadBandFile` reads `demand` as a raw message and drops it when it is not an object, or when `band` or `until` is not a string: `window: none` then prints `band: no 5h window`, never nothing. Pinned in effects' `TestAR98_` with `"weird"`, `[]`, `{"until": 5}`.
5. **Pins for the surviving mutants that matter:** a row arriving after a bare queued row is promoted; the weekly end's `leave` and its log line; the weekly end's `sit.stop` value, whichever the existing weekly path writes; plan's clock regex anchored; a non-ENOENT read error on the plan's file reads as no line. The rest are equivalent or out of scope.

## 3. Interfaces

No exported signature moves; `effects.BandFile` is unchanged.

## 4. Findings that name this row

F1 to F4, ruled in §2.

## 5. Forbidden reads

The parent packet's.

## 6. Report (under 12 lines)

Commits; the acceptance lines; forks; anything unmet.
