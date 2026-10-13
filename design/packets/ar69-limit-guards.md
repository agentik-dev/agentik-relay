# Packet AR69: the guards cap the pauses

| item | value |
|---|---|
| from | `design/usage-limit-resume-r1.md` §5 packet 5, its first part (§2's count, same reset, probe, later reset; §4 test 4's first sentence); `NICE_TO_HAVE.md`'s AR65 (probe C) and AR67 notes; `design/relay-r1.md` §1, §5, §11 |
| pin | agent-relay `main` after AR68's merge (AR68 moves `night.go`; this row depends on AR67 and AR68); worktree `.claude/worktrees/ar69-limit-guards`, branch `ar69-limit-guards` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet ./internal/night ./internal/stream`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/stream/limit.go` (`Limit` and `Tally.Limit` only), `internal/stream/ar69_test.go` (new), `internal/stream/ar63_test.go` (its `Limit` literals keyed, only), `internal/night/pause.go`, `internal/night/wake.go` (`remainder` only), `internal/night/night.go`, `internal/night/ar69_internal_test.go` (new), `internal/night/ar69_test.go` (new), `design/relay-r1.md` (§1, §5, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^Test(AR69_\|AR68_\|AR67_\|AR66_\|AR65_)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR69_\|ok )"` reads 6; `go test ./internal/stream -run "^Test(AR69_\|AR63)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR69_\|ok )"` reads 2; `go test ./cmd/relay -run "^TestAR59_4_Gofmt" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The record, §2: `pauseFor` "stops the night `usage-limit` at once: [...] after 12 pauses; when a wake's sitting is refused with a known reset not after the one waited out; or while draining." "An unknown limit with no reset waits 30 minutes (source `fallback`), and that wake's sitting is the probe [...]; a probe refused again with no reset stops the night." "A later reset among them moves the wake later." §4 test 4: "The thirteenth pause and the same reset twice stop with each detail." AR65's note: a killed sibling's "own weekly hit is lost, so the night stays paused until the five-hour wake" (probe C). AR67's: "Re-pauses are unbounded"; "The floor can exceed the row's budget."

The code at `5fbc523` (AR68 moves `night.go`): `pauseFor`'s stops (`internal/night/pause.go:54-66`); `collect` writes a paused row's late held line paused, reading nothing else (`internal/night/night.go:1061-1068`); the pause sets `wake` once (`:1106`); the floor (`internal/night/wake.go:40-41`); `within` drops an event's `resetsAt` not after the hit (`internal/stream/limit.go:94`, `:232-234`), so a wake's sitting refused with the reset it waited out reads no reset.

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar69.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message.

1. **Act 1, the named reset** (`stream`). `Limit.Named` is the reset the stop named before `within`: the event's `resetsAt`, in the hit's zone, when it sent one; else `Reset`. Without it the same-reset stop never fires. The exit line is unchanged.
2. **Act 1, the guards** (`pauseFor`, pure). `pauseIn` gains `Count`, the night's pauses before this hit, and `Waited`, the reset the night last woke from (zero: not woken, or woken from a fallback). After the model's no-reset stop and before `--until`, in this order, it stops: with `Count` above 0, `Waited` zero and no `Reset`, `refused again after the 30-minute wait, with no reset` (the probe; entry 23's proposed "second unrecognized limit"); with `Waited` set and a non-zero `Named` not after it, `the reset <Named> is not after the one waited out, <Waited>` (RFC3339); at `Count` `MaxPauses` (12), `12 pauses tonight, the most a night takes`. Each keeps its wake and source, as `--until`'s does. A time-only text's rolled reset stops at `--until` or 6 h.
3. **Act 1, the floor** (`remainder`): 10 turns and $5, or the row's own `turns` and `usd` when less (the queue keeps both positive); relay-r1 §1 records this as an amendment to the record's §2 floor.
4. **Act 2, the night's inputs.** The pause passes `pauses` and `waited`, keeping its `Reset` (zero for a fallback), which `wakeUp` makes `waited` after `OnWake`.
5. **Act 2, a killed sibling's own hit.** In `collect`, a paused row's late outcome whose held line has a kind, the night not stopped, asks `pauseFor` with its pause's inputs (`Count` less this pause). A stop: the line written without `limit: paused`, the stage unlocked, `OnSittingFailed` `usage-limit`, that detail; the halt follows. A pause from a reset whose wake is later than the night's: the wake and the kept reset move there, and a second `pause` line (`rows` the sibling, its kind, `resets_at`, `source`, the new wake, the same `count`) sends its notice. Else as today. A sibling never raises the count.
6. **Tests.** stream: `TestAR69_1_NamedReset` (an event `resetsAt` before the hit: `Reset` zero, `Named` it; after: both; a text alone: `Named` is `Reset`). night, internal: `TestAR69_2_PauseGuards` (`Count` 1, `Waited` zero: no reset stops with the probe's detail, a reset pauses; `Waited` R: `Named` R stops with that detail, R+1h pauses, none takes the fallback; `Count` 12 stops, 11 pauses; `Count` 0: TestAR65_1's cases hold), `TestAR69_3_FloorCapped` (8 turns, $3, nothing spent: 8 and 3; TestAR67_2's cases hold). Nights on TestAR65_3's world: `TestAR69_4_ThirteenthPause` (r1's 13 parts each a refused `five_hour` event, `resetsAt` T+k h, then `API Error: 429`, exit 143; the clock to each wake: 12 `pause` lines, counts 1 to 12, 12 `wake` lines, 13 spawns, `stopped:usage-limit` with the count's detail, r1 failed usage-limit, unlocked, its tree kept). `TestAR69_5_SameReset` (part 2 refused with part 1's `resetsAt`: at the wake `stopped:usage-limit`, that detail, one `pause` and one `wake` line, two spawns; the probe, both parts `API Error: 429` alone: one `fallback` `pause` line, the clock 30 min on, the probe's detail, two spawns). `TestAR69_6_SiblingsOwnLimit` (`--parallel 2`, r1 `ar67Limited`; r2 silent until r1's `pause` line, then its own refused event, exit 143: `seven_day`, `stopped:usage-limit`, `a weekly limit`, the clock unmoved, no `wake` line, both failed usage-limit, unlocked, trees kept; `five_hour` at T+2h, a second `pause` line (`[r2]`, count 1, wake T+2h2m), no `wake` line at T+1h2m, one at T+2h2m, both green, `queue-empty`; at T+30m, one `pause` line, the wake T+1h2m).
7. **The record, act 3.** relay-r1 §1: the three stops, the sibling's hit, the floor's amendment; §5: the second `pause` line; §11: the tests. The NICE_TO_HAVE notes this row takes gain `(AR69)`; those it leaves name AR70 or AR71.
8. **Left to later packets.** AR70 (the sibling's verdict) gates a sibling killed in its gate again at the wake, its stage held through the pause (probe B), with an outcome left uncollected, N4's pair (`Paused` to green, red or failed), a killed sibling's broken line or own `auth` stop, and test 4's "spawned apart"; until then AR67's `killed in its gate` stop stands. AR71 (a model's pause) pauses only a model's rows on its limit with a reset, with AR64's N2 and N3; until then it pauses every row (AR65). No packet takes a killed sibling's remainder: its part 1's turn tally and its `spawn` line's `usd` stand (AR51 ruling 1, AR67 ruling 2). Packet 6 and `Night.tla` stay out. With the pauses capped, AR65 to AR69 may install together; AR70 and AR71 refine a safe stop and a safe pause.

## 3. Interfaces (signatures, not prose)

`stream`: `Limit.Named time.Time`. `night`: `const MaxPauses = 12`; `pauseIn.Count int`, `pauseIn.Waited time.Time`. Red on the pin: `_1`, `_2` (undefined), `_3` (10 and 5); at act 1's head, `_4` (a 13th `pause` line), `_5` (a fallback pause, no stop), `_6` (paused to T+1h2m, one `pause` line).

## 4. Findings that name this row

The refuted packets' note (2026-10-11): "the wake also waits for packet 5 (the guards), since without it nothing caps the pauses." AR65's and AR67's, in section 1.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: the TestAR69 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
