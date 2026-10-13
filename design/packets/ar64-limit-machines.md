# Packet AR64: the pause in the two machines

| item | value |
|---|---|
| from | `design/usage-limit-resume-r1.md` §5 packet 2 (§2's pause, wake and halt; §3's `internal/relay` seam); `design/relay-r1.md` §1 |
| pin | agent-relay `main` after AR63's merge; worktree `.claude/worktrees/ar64-limit-machines`, branch `ar64-limit-machines` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet ./internal/relay`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/relay/types.go`, `internal/relay/machine.go`, `internal/relay/night.go`, `internal/relay/actions.go`, `internal/relay/machine_test.go`, `internal/relay/night_test.go`, `internal/relay/actions_test.go`, `internal/relay/ar64_test.go` (new), `design/relay-r1.md` (§1, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/relay -run "^Test(AR64_\|StepMatrix\|NightMatrix\|EveryStopHasReason\|Actions\|AR17D_0_)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR64_\|ok )"` reads 3; `go test ./internal/night -run "^TestAR17D_0_" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./cmd/relay -run "^TestAR59_4_Gofmt" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The record, §3: "`internal/relay/machine.go:5-12`, `types.go:220-224`, `:263-278`, `night.go:22-30`: `Paused` (`Limited` from `Exited`; `Spawn`, `Halt`), `paused:usage-limit` (not terminal), `OnLimited`, `OnWake`." §2: "Else the night steps `OnLimited` to `paused:usage-limit`." "A window or unknown limit kills every other running row as today, and each comes back paused; a model's limit pauses only that model's rows." "While paused the loop picks no row of the paused scope." "`relay stop` reads `stopped:owner-stop` at the next poll; `relay kill` or a signal, `stopped:owner-kill`". "A night that ends paused halts each paused row `failed: usage-limit` (`Step(Paused, Halt)`)". "The night steps `OnWake`, writes a `wake` line and checks `--until` and `--max-usd` [...], not `--max-sittings`. A failed check stops the night and halts the paused rows." It stops `usage-limit` at once "while draining". §5: "**The machines** (`internal/relay`): the states, the events, the matrix tests; relay-r1 §1."

The code (unchanged since `bad2001`): the sitting table (`internal/relay/machine.go:5-12`); `States` (`types.go:31`), `Events` (`:60`), `NightStates` (`:245`, three live states, then a stop per reason), `NightEvents` (`:281`); `NightStep`'s kill and failure rules name planning, running and draining (`night.go:22-30`). The tests pin the counts: `10*15` (`machine_test.go:54`); 33 states and 14 events (`night_test.go:77`); the live states (`:110`, `:124`); a row per state (`actions_test.go:35`). `RunRow` steps its gate from a literal `relay.Exited` (`internal/night/run.go`, `relay.Step(relay.Exited, ev)`).

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar64.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message. The loop steps none of the new pairs yet: the account still stops the night.

1. **Act 1, the sitting.** `Paused State = "paused"`, live, after `Exited` in `States`; `Limited Event = "limited"`, last in `Events`. The table gains `Exited: Limited → Paused` and `Paused: Spawn → Running` (a continuation), `Halt → Failed` (the loop sets `usage-limit`). Every other event on `Paused`, and `Limited` on any other live state, is `ErrIllegal`. `Actions(Paused)` is `[ActLedger]`, the `sitting` line; the notice is the night's `pause` line's. A sibling gated again at the wake (packet 5) needs no pair: `RunRow` steps from a literal `Exited`.
2. **Act 1, its tests.** `TestStepMatrix` walks 11 × 16 with the three pairs; `TestActions` gains a paused row. `TestAR64_1_SittingPause`: the three pairs; each other event on paused refused; `limited` refused on every live state but exited, `ErrTerminal` on the terminal ones; `Terminal(Paused)` false; `Actions`.
3. **Act 2, the night.** `NightPaused NightState = "paused:usage-limit"`, live (`NightTerminal` false, `StopReasonOf` not ok), fourth in `NightStates`; `OnLimited NightEvent = "limited"` and `OnWake NightEvent = "wake"`, last in `NightEvents`. Legal: running, `limited` → paused. Paused: `limited` → paused (a later hit); `wake` → running; `sitting-green`, `sitting-red` → paused (rows outside a model's scope run on); `owner-stop` → `stopped:owner-stop` (no draining while paused); `owner-kill` → `stopped:owner-kill`; `until` → `stopped:until-reached` and `budget` → `stopped:budget-reached` (a check while other rows run); `sitting-failed` with each of `SittingFailures` → `stopped:<r>` (every stop `pauseFor` names, the halt at the wake). Paused refuses `planned`, `queue-empty`, `count`, `blocked`, `origin-moved`, `tmux-gone` and `drained`: a pause holds rows, and continuations sit outside `--max-sittings`. Planning and draining refuse `limited` (a draining night's hit is `sitting-failed`); only paused takes `wake`. `StopReasons` stays thirty.
4. **Act 2, its tests.** `TestNightMatrix`: 34 states, 16 events, `failing` holding paused, its count comment gaining `33 until AR64`; `TestEveryStopHasReason`'s live lists hold paused. `TestAR64_2_NightPause`: each pair of ruling 3 and each refusal; the state's string; the walk running, `limited`, `wake`, `until` ends `stopped:until-reached`.
5. **The record, in act 2.** relay-r1 §1: sitting states (eleven) gain `paused` (the account's limit paused the night on this part: no child; its tree, stage lock and `Staged` kept); events (sixteen) `limited`; the table an exited cell and a paused row; night states `paused:usage-limit` with ruling 3's pairs, and one sentence: the loop steps none of them until the pause lands (usage-limit-resume r1 §5 packet 3). §11: the two tests.
6. **Not done.** `pauseFor`, the `pause` and `wake` lines, the continuation (packets 3 to 5); `AccountStop` and `SittingFailures`; `Night.tla` (the record's §6); the readers (packet 6).

## 3. Interfaces (signatures, not prose)

`relay`: `const Paused State = "paused"`, `const Limited Event = "limited"`, `const NightPaused NightState = "paused:usage-limit"`, `const OnLimited NightEvent = "limited"`, `const OnWake NightEvent = "wake"`. Tests `func TestAR64_<n>_<Name>(t *testing.T)`; red on the pin: each (undefined names).

## 4. Findings that name this row

The record's §1: five ledger hits, each stopping the night `usage-limit` with every row skipped. The owner's word (observer, 2026-10-10): "sessions and relay runs resume by themselves after a usage limit (the 5-hour window), instead of stopping."

## 5. Forbidden reads

Transcripts; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line with its count, `go vet`'s exit, `make doclint`'s exit and `make hygiene`'s last line. Bullets: the TestAR64 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
