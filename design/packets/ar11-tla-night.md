# Packet AR11: the night modeled (TLA+, the storm found, the run guard)

| item | value |
|---|---|
| from | `design/relay-r1.md` §1, §3, §6 to §8, §11, §15; `design/review-r6.md` §3 to §5; `design/review-adv6.md` ADV6-1, -2, -7; `internal/night/plan.go`, `run.go`, `night.go`, `bound.go`; `internal/gate/gate.go`; `internal/effects/shell.go`, `spawn.go`, `procs.go`; the handover of `history/sittings/2026-09-23-7.md`. Never a ledger |
| pin | agent-relay `main` after packets-25 (cut off `main`, no `base`); worktree `.claude/worktrees/ar11-tla-night` |
| wall | this repo only. No push, `--amend`, rebase or device; the network ONCE, `make tla-tools` fetching the jar into the gitignored `design/tla/.tools/`; TLC always `-workers 4` over §3's small constants; `relay plan` only with `--rows` and `RELAY_MEASURE_DEPTH=1` under a temp `XDG_STATE_HOME`; `relay run` never; `go test ./...` only inside `make check`; the pinned scripts never edited |
| may touch | `design/tla/`, `.gitignore`, `Makefile`, `design/relay-r1.md`, `cmd/relay/main.go`, `cmd/relay/*_test.go`, `internal/night/night.go`, `internal/night/*_test.go`, `internal/hooks/*.go`, `history/sittings/*.md`, `history/sessions/NEXT` (removed if present). Nothing else |
| acceptance | `make tla 2>&1 \| tail -1` reads `tla: ok`; `grep -c 'is violated' design/tla/storm-trace.txt` reads 1; `grep -c 'is violated' design/tla/storm-run-trace.txt` reads 1; `go test ./cmd/relay -run 'TestAR11_' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads 1 or more; `go test ./internal/night -run 'TestAR11_' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads 1 or more; `go test ./internal/hooks -run 'TestAR11_' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads 1 or more; `grep -c 'Night.tla' design/relay-r1.md` reads 2 or more; `grep -rl '(filled at close)' history/sittings \| wc -l` under 2; `test -e history/sessions/NEXT; echo $?` reads 1 |
| budget | 100 turns, 130k context, `claude-opus-5-5`; one commit per act (0 to 5) plus the charter's; `make check` is the verify line, never acceptance; checkpoint at 80%; the charter is the first free `history/sittings/<date>-<n>.md`, closed before the last commit |
| verify | `make check` |

## 1. What the source says (quoted)

The owner, 2026-09-23 12:35: "opportunities for TLA+ for timing and edge conditions that might cause fork storms (those are incredibly bad and we need to lock that down)". §11: "every invariant is one named test, and a runtime guard too where it can be". R6 §3 C4: "a relay child runs `relay run` ... Bound: NONE in the binary".

## 2. Rulings

Decide inside the design, record each choice in the charter, at most ONE fork. One commit per act with its design clause; act 0 first.

0. **Tools and skeleton.** `Makefile`: `JAVA ?= /opt/homebrew/opt/openjdk/bin/java` (else PATH's; `/usr/bin/java` is a stub with no runtime), `TLA_JAR = design/tla/.tools/tla2tools.jar`; `make tla-tools` fetches the jar once (`curl -fsSL https://github.com/tlaplus/tlaplus/releases/latest/download/tla2tools.jar`); `make tla` never fetches: jar absent, it prints that line and exits 2; neither is in `check`. `.gitignore` gains `design/tla/.tools/`. `design/tla/Night.tla` (plain TLA+, no PlusCal; one action per row of §1's tables) and `Night.cfg` with §3's constants; TLC runs clean from `design/tla/` (`$(JAVA) -XX:+UseParallelGC -cp .tools/tla2tools.jar tlc2.TLC -workers 4 -config Night.cfg Night.tla`); terminal states stutter (`Done`), never `-deadlock`; under 120 s here, else shrink a constant.
1. **Safety.** §3's nine invariants, each under a comment naming its Go guard and test (§11); `Night.cfg` lists all nine. Booleans per row for the ledger, never a sequence; the load a bounded counter.
2. **Liveness.** `FairSpec == Spec /\ WF_vars(Poll) /\ WF_vars(Stall) /\ WF_vars(BreakerPoll) /\ WF_vars(NightStep)`; `LoadFair == FairSpec /\ WF_vars(LoadDown)` is the assumption `OpenBreakerEventuallyCloses` needs (`Night.cfg`: `SPECIFICATION LoadFair`); `OpenBreakerEventuallyResolves` (closed or tripped at `MaxWait`) and the other two hold under `FairSpec` alone.
3. **The storm.** `Storm.cfg`: `Guard = FALSE`, `LinesMayPlan = TRUE`, the one invariant `NoNestedMeasure`: TLC must print `Error: Invariant NoNestedMeasure is violated.` `Storm-run.cfg`: `RunGuard = FALSE`, the one invariant `NoNestedSpawn` (C4, ADV6-1): violated. Each trace is committed as `design/tla/storm-trace.txt` and `storm-run-trace.txt`, cut to the lines from `Error:` to the last state (TLC's path lines above them are a hygiene red). `make tla` asserts §3's thirteen names are defined, runs the three configs, asserts clean, violated, violated by grep on TLC's output (exit codes are not enough), ends `tla: ok`; else exits 2 naming the config.
4. **The run guard (C4).** `relay run` under `RELAY_MEASURE_DEPTH` or `RELAY_ROW` exits 2 before any stage (`nested: never under a measurement or a sitting`); `night.Run` returns `ErrNested` for any caller; the child's PreToolUse guard denies `relay run`, and `relay plan` without `--rows`, when `RELAY_ROW` is set. No new stop reason: a pre-check, like an invalid plan. `TestAR11_4_NestedRunRefuses` (exit 2, zero spawns; the verb's other tests clear the depth with `measuring`), `TestAR11_4_NightRunNestedRefuses`, `TestAR11_4_HookDeniesNestedRun`. `Night.cfg`: `RunGuard = TRUE`, the design of record.
5. **The design.** `design/relay-r1.md` gains `## §16 The model`: files, constants, each invariant and property with its Go guard and test, the storms, `make tla`, and the rule: a change to plan, measure, spawn, watch, gate or merge updates `Night.tla` in the same packet; a new invariant in Go is one in the model. §11 gains the `TestAR11_4_` names; §15's recursion clause gains the run guard. The gaps, in §16: ar10's 1 (a fresh branch reads as landed) is `BranchTipMovedBeforeLanded` here, its Go change (landed and satisfied read main's first-parent merges, never bare ancestry) is AR13's, the fixes packet cut from the two reviews; ar10's 2 (an orphan holding stdout; ADV6-7) is `NoOrphanAfterExit` here, AR12 asserts the wall bounds it, the read end's close is AR13's; ar10's 3 (the budget before the probe) is AR13's test.

## 3. Interfaces (signatures, not prose)

- CONSTANTS (`Night.cfg`): `Rows = {r1, r2}`, `MaxProcs = 5`, `MaxLoad = 5` (units of cores), `OpenAt = 4`, `CloseAt = 2`, `MaxWait = 2` (breaker polls), `StallAt = 2` (watchdog polls), `Wall = 3`, `MaxDepth = 3`, `Guard = TRUE`, `RunGuard = TRUE`, `LinesMayPlan = FALSE`; `CONSTRAINT Bounded`.
- VARIABLES: `row` (§1's states, reason), `night`, `procs` (pid ↦ [kind: relay|measure|plan|child|orphan, parent, group, depth, alive, holdsStdout, spawnedOpen]), `measuring`, `memo`, `tip`, `cut`, `mainMerges` (first-parent merges), `load`, `breaker`, `waited`, `stall`, `checks` (green|red|unrun|same-as-main), `anchorLoaded`, `written` (plan, spawn, exit, sitting).
- ACTIONS: `PlanValidate`, `Measure(r)`, `MeasureDone(r)`, `LineStartsPlan(p)` (only `LinesMayPlan`), `Spawn(r)`, `NestedRun(c)` (refused under `RunGuard`), `Progress(r)`, `Poll(r)`, `Stall(r)`, `WallExpires(r)`, `ChildExit(r)`, `Sweep(r)`, `Gate(r)`, `Merge(r)`, `LoadUp`, `LoadDown`, `BreakerPoll`, `BreakerTrip`, `OwnerStop`, `NightStep`, `Done`.
- INVARIANTS: `NoNestedMeasure` (every measure's depth is 1: the relay at 0 started it), `OneMeasurePerRoot`, `NoSpawnWhenOpen` (no live child has `spawnedOpen`), `NoOrphanAfterExit` (sitting written ⇒ no live pid in the child's group), `UnrunNeverPasses`, `NoSameAsMainAgainstLoaded` (same-as-main ⇒ `anchorLoaded` false), `LandedNeverMeasured`, `BranchTipMovedBeforeLanded` (landed ⇒ `tip # cut /\ r \in mainMerges`), `NoNestedSpawn` (a child's parent is the relay).
- PROPERTIES: `StalledChildEventuallyKilled` (stall at `StallAt` leads to the child dead), `OpenBreakerEventuallyCloses == [](breaker = "open" => <>(breaker = "closed"))`, `OpenBreakerEventuallyResolves`, `EveryRunnableRowEventuallyGatedOrParked` (ready leads to a terminal state).
- Go: `runVerb` reads `effects.Nested(getenv) || getenv("RELAY_ROW") != ""` first; `night.ErrNested`, read in `night.Run` through `fx.Getenv`; the hooks guard `nested-run`. `Makefile`: `JAVA`, `TLA_JAR`, `tla`, `tla-tools`.

## 4. Findings that name this row

Baseline on `main` at c4a0777, 2026-09-23: `/usr/bin/java` has no runtime; `/opt/homebrew/opt/openjdk/bin/java` is openjdk 26.0.2.1; no `design/tla/`. `runVerb` and `night.Run` read `Nested` nowhere (ADV6-1: `spawns=1` under depth 1). TLC's lines: `Error: Invariant <name> is violated.`; `Model checking completed. No error has been found.` The `(filled at close)` count is 1 (packets-20's charter); no NEXT.

## 5. Forbidden reads

Every other repo; `~/.claude/projects/`; the real state root; any ledger or transcript; example-mac's `history/relay/`; the packets `a3-*` to `ar10-*`; `.claude/worktrees/`; any file over 8 KB whole unless in `may touch`.

## 6. Report (capped)

The table: each acceptance line, its exit and last line. Bullets: TLC's time and states per config; each storm trace's depth; the guard's call sites; what was left undone and where it goes; the one open fork with its consumer. Last line: the branch and its head sha.
