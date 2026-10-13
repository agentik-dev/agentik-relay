# Packet AR71: a model's limit with a reset pauses only its rows

| item | value |
|---|---|
| from | `design/usage-limit-resume-r1.md` §2 (a model's limit, the paused scope, a later reset), §4 test 4's third sentence, §5 packet 5's rest, §7 (changed); `NICE_TO_HAVE.md`'s AR64 notes (N2, N3), AR69's and the AR71 hold's N1; `DECISIONS.md` entry 12 (the owner's answer, 2026-10-12); `design/relay-r1.md` §1, §5, §11 |
| pin | agent-relay `main` after AR70's merge (AR70 moves `night.go`; this row depends on AR69 and AR70); worktree `.claude/worktrees/ar71-model-pause`, branch `ar71-model-pause` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet ./internal/night`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/pause.go` (`pause`, `pauseFor` and the new `ofModel` only), `internal/night/schedule.go` (`PickOptions` and `PickIn` only), `internal/night/night.go`, `internal/night/ar69_internal_test.go` (its `pause` literals keyed, only), `internal/night/ar71_internal_test.go` (new), `internal/night/ar71_test.go` (new), `design/relay-r1.md` (§1, §5, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^Test(AR71_\|AR70_\|AR69_\|AR67_\|AR66_\|AR65_\|Pick\|OneSittingPerTree)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR71_\|ok )"` reads 8 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The record, §2: "A window or unknown limit kills every other running row as today, and each comes back paused; a model's limit pauses only that model's rows. A later reset among them moves the wake later." "While paused the loop picks no row of the paused scope." §4 test 4: "A model's limit with a reset pauses only its rows." AR64's N2: "Paused takes `until` and `budget` as 'a check while other rows run', but running steps them only once no row runs. Packet 3 should say whether a paused night stops with rows outside a model's scope still running." N3: "in a model's pause, rows outside its scope may still run, so §1's draining ('the running ones finish') does not hold there." AR69's refuter: until AR71 a model's limit with a reset "pauses every row up to 6 h or `--until`".

The code at `dc6bb83` (AR68's head; AR69 and AR70 move `night.go`): the pause kills and pauses every running row (`internal/night/night.go:1102-1114`) and sets the wake outright (`:1117`); the paused loop only polls the wake (`:1385-1391`); the pick runs only while running (`:1392-1414`) and checks no window while paused (`:1488`). At `dcef185`: `pauseFor` pauses a model's limit with a reset (`internal/night/pause.go:45-68`); `Limit.Scope` is the model's word, lowered (`internal/stream/limit.go:125-139`); `PickIn` skips a paused row as waiting (`internal/night/schedule.go:68-71`); the plan's model is the queue row's `Model` (`internal/night/plan.go:1003`).

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar71.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message.

1. **Act 1, the scope** (pure). `pauseFor`'s pause on a model's limit (which pauses only with a reset) carries `Scope`, the Limit's word; any other pause, "" (every row). `ofModel(model, scope)` is true when scope is "", or when the model (the queue requires one), lowered and cut at every rune not a letter or digit, holds scope as one piece (`claude-fable-5-1` holds `fable`; `claude-opus-5-5` and `fablex-1` do not).
2. **Act 1, the hold.** `PickOptions.Hold`: a row it holds is never picked and counts as waiting, as a paused row does (never doomed, never blocked).
3. **Act 2, the pause's scope.** The night keeps its paused scope: every row, or a set of model words. A pause with `Scope` kills and pauses, beside the hit row, only the running rows `ofModel` it (the queue row's `Model`); the others run on. A later hit while paused (a row outside the scope, or AR69's sibling): a model's adds its word to a scope of model words, and leaves an every-row scope every row (N1: a window's pause, then a sibling's model hit, stays every row); any other makes the scope every row, killing and pausing the running rows as a window's pause does. The wake and the kept reset move only later; a fresh hit's `pause` line counts one more pause, a sibling's keeps the count (AR69). The `pause` line gains `scope` (the word; "" every row). The wake clears the scope.
4. **Act 2, the loop while a model's scope holds** (N2). It picks as a running night does: none once `--until`, `--max-sittings` or `--max-usd` is reached; `PickIn`'s busy the running and the paused rows (each keeps its slot and tree, so the wake's continuations stay within `--parallel`); `Hold` the scope's rows. While paused it steps none of `until`, `count`, `budget`, `queue-empty`, `blocked` or an orchestrator's cut, and checks no window (N1): the paused rows wait for the wake, which checks `--until` and `--max-usd` (AR67; `pauseFor` keeps the wake before `--until`). Paused for every row, the loop picks nothing, as today.
5. **Act 2, the owner** (N3), no code: `relay stop` while a scope holds steps `stopped:owner-stop` at once (AR66), and the rows outside it finish before the `stop` line as draining's do, since the loop collects every running row before it ends; a test pins it.
6. **Tests.** Internal: `TestAR71_1_ModelScope` (`ofModel`'s cases; `pauseFor` on a model's limit with a reset: `Scope` `fable`, a window's ""; `PickIn` with `Hold` on a ready row: not picked, its dependent waits, not blocked). Nights on TestAR67's world, `--parallel 2`, r1's queue `Model` `claude-fable-5-1`, r2 and r3 opus; r1's part 1 an `is_error` result `You've reached your Fable limit · resets 2am (UTC)`, exit 1, part 2 clean; r2's child held until r1's `pause` line; no case writes a `kill` line. `TestAR71_2_ModelPause` (one `pause` line: `[r1]`, kind model, scope `fable`, wake 02:02; before the clock moves, the night paused, r2 green, then r3 spawned after r2's `sitting` line and green; at 02:02 one `wake` line `[r1]`, r1's part 2, all green, `queue-empty`, 4 spawns). `TestAR71_3_ScopeEdges` (r2's child a refused `five_hour` event, `resetsAt` 03:00, exit 143: a second `pause` line `[r2]`, count 2, scope "", wake 03:02; at 02:02 no `wake` line, r3 unspawned; at 03:02 one `wake` line `[r1 r2]`, all green; N1, on TestAR69_6's night apart from this world (r1's window pauses the night and kills r2, so a `kill` line is written), r2's queue `Model` `claude-fable-5-1`, r2 killed and back with the Fable `is_error` result `resets 2am (UTC)`: a second `pause` line `[r2]`, kind model, count 1 (a sibling keeps it), scope "", the wake r1's, and at the wake one `wake` line `[r1 r2]`, both green; `relay stop` while r2 runs, its child held until the `owner-stop` line: `stopped:owner-stop`, r2 green, its `sitting` line before the `stop` line, r3 never spawned, r1 failed usage-limit, unlocked; `--max-sittings 2`: r3 never picked, r1's part 2 at the wake, then `count-reached`, r3 skipped).
7. **The record, act 3.** relay-r1 §1: the scope, the picks while paused, a later hit, N2's and N3's answers; §5: the `pause` line's `scope`; §11: the tests. N2 and N3 gain `(AR71)`.
8. **Left.** One wake serves the night: a model's rows paused beside a later window's reset wait for the later wake, as the record's §2 says. Status and report show the scope in packet 6; `Night.tla` stays out.

## 3. Interfaces (signatures, not prose)

`night`: `pause.Scope string`; `func ofModel(model, scope string) bool`; `PickOptions.Hold func(id string) bool`; the `pause` line's `scope`. Red on the pin: `_1` (undefined); at act 1's head, `_2` and `_3` (r2 killed at r1's pause, a `kill` line, r2 paused; the N1 case's scope narrowed to `fable`).

## 4. Findings that name this row

AR69 ruling 8 names AR71 (a model's pause) with AR64's N2 and N3; the AR71 hold's N1, folded at ruling 3 and `TestAR71_3`; `DECISIONS.md` entry 12 rules it (the five-hour window still pauses every row); the record's §7 (changed) and AR69's refuter note: section 1.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: the acceptance line's count, `go vet`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: the TestAR71 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
