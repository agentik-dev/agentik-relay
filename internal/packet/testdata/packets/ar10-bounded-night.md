# Packet AR10: the night bounded (fail early, scope, memo, budget, breaker, watchdog, invariants, sweep)

| item | value |
|---|---|
| from | `design/relay-r1.md` §5 to §9 and §11; `internal/night/plan.go`, `stage.go`, `run.go`, `night.go`; `internal/effects/shell.go`, `spawn.go`; `internal/gate/gate.go`; the ar9 packet §2 item 1. The brief is quoted in §1; the builder never reads it or a ledger |
| pin | agent-relay `main` after `ar9-queue-and-resync` merged (cut off `main`, no `base`); worktree `.claude/worktrees/ar10-bounded-night` |
| wall | this repo only. No push, `--amend`, rebase, device or network; `relay plan` only with `--rows` and `RELAY_MEASURE_DEPTH=1` under a temp `XDG_STATE_HOME`; `relay run` never; `go test ./...` only inside `make check`; `scripts/doclint`, `scripts/hygiene` never edited (conventions §6) |
| may touch | `relay/queue.yaml`, `design/relay-r1.md`, `cmd/relay/main.go`, `cmd/relay/main_test.go`, `internal/effects/*.go`, `internal/night/*.go`, `internal/gate/*.go`, `internal/packet/*.go`, `internal/relay/*.go`, `history/sittings/*.md`, `history/sessions/NEXT` (removed if present). Nothing else |
| acceptance | `go test ./internal/night -run 'TestAR10_' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads 12 or more; `go test ./internal/effects -run 'TestAR10_' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads 4 or more; `go test ./internal/gate -run 'TestAR10_' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads 2 or more; `go test ./internal/packet -run 'TestAR10_' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads 1 or more; `go test ./internal/effects -run 'TestAR2_6_' -count=1 -v 2>&1 \| sed -n 's/.*Child (\([0-9.]*\)s)$/\1/p' \| awk '{print ($1 < 2.0) ? 1 : 0}'` reads 1; `grep -c 'TestAR10_' design/relay-r1.md` reads 9 or more; `grep -c '^  - id:' relay/queue.yaml` reads 1; `t=$(mktemp -d) && RELAY_MEASURE_DEPTH=1 XDG_STATE_HOME="$t" go run ./cmd/relay plan --rows ar10-bounded-night > "$t/o" 2>&1; echo "$? $(grep -c 'would open with 1 row; valid$' "$t/o")"` reads `0 1`; `grep -rl '(filled at close)' history/sittings \| wc -l` under 2; `test -e history/sessions/NEXT; echo $?` reads 1 |
| budget | 120 turns, 130k context, `claude-opus-5-5`; one commit per act (0 to 9) plus the charter's, each act's design clause in its commit; `make check` is the verify line, never an acceptance line; checkpoint at 80%; the charter is the first free `history/sittings/<date>-<n>.md` on the branch, its `(filled at close)` lines filled before the last commit |
| verify | `make check` |

## 1. What the source says (quoted)

§11: "every invariant is one named test, and a runtime guard too". The brief, D1: `baseline` "runs EVERY acceptance line of a row on main ... no memo ... no cap on the total, no scope to runnable rows, no load check". D5: "a plan started by a measurement or a test never measures". D4: "A baseline measured while the breaker would have been open is not a baseline".

## 2. Rulings

The owner's standing word: decide inside the design, record each choice in the charter, escalate at most ONE real fork. Each act: its test fails first, the code, its clause in `design/relay-r1.md` (a new `## §15 Bounded work` naming each guard; §11 gains the act's test names), one commit. Act 0 first.

0. **Recursion guard, the queue.** `OSShell.Run` sets `RELAY_MEASURE_DEPTH=1` for every line; `ChildEnv` adds it beside `RELAY_ROW`. `BuildPlan`, `BaselineOnMain` and the gate read it: set, every check is `unrun: nested` and `Print` says so. Tests that run the binary or `run()` set it. `packet.Parse` refuses `relay (plan|run|gate)` without `--rows` and `go test` without a package and `-run`. `TestAR10_0_NestedPlanMeasuresOnce`: a fixture queue whose one line runs the built binary's `plan --rows` on itself measures once; the inner ledger holds only `nested`. The commit also drops the landed ar9 row and adds this one (`id`, `packet`, `budget: {turns: 120}`, `merge`, `device`; no `depends_on`), per the ar9 rule.
1. **Fail early.** Every validation before any shell line; one problem: no baseline at all, checks `unrun: invalid`, exit 2; `FakeShell.Calls()` empty.
2. **Scope.** Only `ready` rows measured; `pending` reads `unrun: pending`; a row merged into `main` or with its own green line is `landed`: checks from its last `plan` line, else `unrun: landed`. `Print` ends `measured N lines, skipped M`; the `plan` line carries both.
3. **Memo.** `<StateDir>/baselines.json`, keyed (root, HEAD, line): exit, last, status, load, ts; written after each measurement (temp file, rename); `unrun` never stored; another head's keys dropped. Read by `plan`, `BaselineOnMain`, `MergeIfGreen`. A second plan at the same head makes no shell call; a moved head measures again.
4. **Bounded.** One measurement at a time per root (a mutex per root); `--measure-seconds` (600) is the whole plan's budget; a line starts only inside it, under the lesser of 900 s and the remainder; past it, `unrun: budget` and the stop line counts them. Time moves through the clock seam.
5. **Breaker.** Before every measurement and spawn, the 1-minute load against the cores: above 4x it opens (poll 30 s up to 10 min through the clock); still open, a spawn is `stop: load` (a pre-check: exit 2, no `child_exit`), a baseline `unrun: load`; it closes under 2x. Open and close: `breaker` ledger lines and `breaker: {opens, wait_s, load}` on the sitting and plan lines. D4: a RED baseline measured above 2x cores is `unrun: loaded`, never a same-as-main anchor; a green one stands.
6. **Watchdog.** In `RunRow`, every 60 s through the clock: the worktree's HEAD, its newest mtime outside `.git`, the child's CPU seconds. All three unchanged for `--stall-seconds` (2700): the group killed, the row red, `stop: stalled`, `stall: {head, mtime, cpu_s, at}` on the line. A row's wall is never zero: one test.
7. **Invariants.** `relay.Invariants(Facts)`, pure: `commit == (head_after != head_before)`; `clean` equals a fresh `git status`; every dependency satisfied per §3; one live child per tree; verify status in {green, red, unrun, skipped}; `unrun` never passes. `sitting.write`, `gate.Run`, `gate.Hand` call it; a violation writes the line with `stop: invariant` and `invariant: <name>`; the night stops `invariant`.
8. **Orphan sweep.** After `Wait`, the group's survivors are killed and re-read, three rounds; `orphans: n` on the line, n > 0 red; a survivor after three is `stop: orphans` (D6). `OSShell.Run` sweeps the same way.
9. **The wait.** `OSShell.Run` returns within 1 s of the shell's exit whatever holds the pipes; the 5 s `WaitDelay` goes.

## 3. Interfaces (signatures, not prose)

- `effects`: `MeasureDepthVar`; `Nested(getenv) bool`; `Load interface { Load() (avg1 float64, cores int, err error) }` (OS via `sysctl`; a fake series); `Procs interface { Newest(dir) (time.Time, error); CPUSeconds(pid) (float64, error); Group(pgid) ([]int, error); KillGroup(pgid) error }`; `Clock` gains `After(d)`, `StepClock{T}.Advance(d)` fires waiters; `Effects` gains `Load`, `Procs`; `Process` gains `Pid() int`.
- `night` (`bound.go`): `Memo{Get, Put, Save}`; `Breaker{Open, Close float64; Poll, Max time.Duration}.Wait(ctx, load, clock) (BreakerStats, bool)`; `Watchdog`; `Options.MeasureSeconds`; `Plan.Measured, Skipped, Breaker`; `StateLanded`; `RunOptions.StallSeconds`.
- `relay`: `StopReason` adds `Stalled`, `LoadOpen = "load"`, `Invariant`, `Orphans`, `Budget`; `Facts` (the heads, commit, clean, a fresh status, deps, live children, verify status and pass); `Invariants(Facts) []string`; `Gate.Orphans int`, `Decide` reds it.
- `cmd/relay`: `--measure-seconds N` on `plan` and `run`, `--stall-seconds N` on `run`; `--json` keys `measured`, `skipped`, `breaker`.

## 4. Findings that name this row

Baseline on `main` at f9e967a, 2026-09-23, 16 cores, `vm.loadavg` `{ 6.28 27.38 139.23 }`: every `TestAR10_` line reads 0 in 0 to 1 s; the `TestAR2_6_` line reads 0 (5.0 s); `TestAR10_` in the design 0; the queue holds 9 ids on main, 1 after ar9; the `(filled at close)` count is 2 (packets-20's charter and this packet's sitting, 1 once it closes); no NEXT. The live recursion paths: ar7's acceptance item `run --dry --rows ar8-resync` on the real queue, and `verifyBaseline` running `make check` before any row. `ChildEnv` strips every `RELAY_*`; the spawn already sets `Setpgid` and `Signal` kills the group; Baselines are unrun on a dirty root: commit, then measure.

## 5. Forbidden reads

Every other repo; `~/.claude/projects/`; the real state root under `~/.local/state/`; any ledger or transcript; example-mac's `history/relay/`; the packets `a3-*` to `a8-*` and `ar1-*` to `ar8-*` (ar9 §2 item 1 only); `.claude/worktrees/`; any file over 8 KB whole unless in `may touch` (the design by its §5 to §9 and §11).

## 6. Report (capped)

The table: each acceptance line, its exit and last line on the branch. Bullets: the recursion path the test closed; the memo's hit and miss; the breaker's open and close; the watchdog's stall sample; the invariant names and their call sites; what was left undone and where it goes, and the one open fork, if any, with its consumer. Last line: the branch and its head sha.
