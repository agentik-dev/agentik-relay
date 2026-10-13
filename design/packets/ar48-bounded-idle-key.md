# Packet AR48: `scripts/bounded` takes the breaker's second key

| item | value |
|---|---|
| from | `design/breaker-signal-r1.md` §2 (the rule), §3 (the two env names) and §5 item 2 |
| pin | agent-relay `main` at 51b33bd plus this packet's own commit; worktree `.claude/worktrees/ar48-bounded-idle-key`, branch `ar48-bounded-idle-key` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. No test runs the real `top`, reads the real load or loads the machine; polls run in tenths of a second. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state. No `make check` and no `go test ./...` (a relay verb may be live): `go test ./internal/night` and `make doclint` only; the observer runs the full check before the merge |
| may touch | `scripts/bounded`, `internal/night/ar48_test.go`, `design/readme/source.md`, `README.md`, `design/readme/rendered.sha256`, `history/sittings/*.md`; `Makefile` (line 15's help text only, ruling 6). Nothing else |
| acceptance | `go test ./internal/night -run "^TestAR48_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR48_"` reads 5 |
| budget | 60 turns, 150k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

`design/breaker-signal-r1.md` §2: "loaded at k = the 1-minute load is above k x cores **and** the idle share of the CPU is under `IdleFloor`." "An unreadable idle share leaves the load key alone to decide, as today. The rule can only narrow when the breaker fires; it never fires where today's does not."

§3: "`scripts/bounded` reads `BOUNDED_IDLE` (a fake idle share, as `BOUNDED_LOADAVG` fakes the load) and `BOUNDED_IDLE_FLOOR` (25). Its waiting line on stderr names both numbers."

What was seen: on 2026-09-30 the load read 72.34 on 16 cores, above `make check`'s line of 64, with `top` at 73% idle. `wait_for_load` (`scripts/bounded:137`) waits on the load alone.

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar48.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's test is red on `main`. `DECISIONS.md` stays as it is. **No existing test line changes.** The script stays standard library only, keeps its main guard, and keeps every exit code and the last stderr line `bounded: stop: lock|load|wall`. Both env values are percentages, 0 to 100.

1. **A high idle share runs the command** (act 1). Above `ceiling` x cores, `wait_for_load` reads the idle share once. Above the floor, the load is not confirmed: it writes one stderr line that names the load, the idle share and the floor, and returns, so the command runs at once. At or under the floor it waits as today, and its waiting line names the load, the idle share and the floor. At or under the load ceiling it reads no idle share. `BOUNDED_IDLE` fakes the reader: one percentage, or several separated by commas, read in order with the last one repeating. Test `TestAR48_1_HighIdleRuns`: with `BOUNDED_LOADAVG=100000`, `--load 4` and `BOUNDED_IDLE=70`, the command runs, the exit is the command's, stderr holds the one line with both numbers and no `stop:` line; with `BOUNDED_IDLE=10`, `BOUNDED_LOAD_MAX=0.5` and `BOUNDED_LOAD_POLL=0.1`, the command does not run, the exit is 2, the waiting line names both numbers and the last line is `bounded: stop: load`.

2. **Two polls close it** (act 2). While it waits, each poll reads the load and then the idle share. It stops waiting when the load is at or under the ceiling, as today, or when two polls in a row read the idle share above the floor. Test `TestAR48_2_TwoPollsClose`: `BOUNDED_IDLE=10,60,60` with a short poll runs the command; `BOUNDED_IDLE=10,60,10` with `BOUNDED_LOAD_MAX=0.5` stops `load` and does not run it.

3. **As today without a reading** (act 3). An unreadable idle share leaves the load key alone to decide, and the stderr lines are then today's, byte for byte. Unreadable is: the reader failing or timing out; a value that is not a number, is NaN or is outside 0 to 100; and **a faked load with no `BOUNDED_IDLE`**, which reads nothing, so a test that fakes only the load behaves as it did and never runs `top`. Test `TestAR48_3_UnreadableAsToday`: `BOUNDED_LOADAVG=100000` with `BOUNDED_IDLE` unset, and again with `BOUNDED_IDLE=nan` and with `BOUNDED_IDLE=x`, stops `load` with exit 2 and today's waiting line; a fake `top` placed first on `PATH` that writes a marker file leaves no marker in the first case.

4. **The real reader** (act 4). With no fake load, the idle share is the last `CPU usage:` line of `top -l 2 -n 0 -s 1` on darwin, and two reads of the `cpu` line of `/proc/stat` one second apart elsewhere. `top` runs under a 5 s timeout. The parsing is one pure function for each: text with no `% idle`, a NaN, a comma decimal or a value outside 0 to 100 is unreadable, never misread. Test `TestAR48_4_ParseIdle`: two `CPU usage:` lines give the second line's share; each unreadable case above is unreadable; two `/proc/stat` lines give the share of their difference. How the test reaches the parsers is yours; it never runs the real `top`.

5. **One floor in both places** (act 5). The script's default floor is one named constant, 25, the same number as `night.DefaultIdleFloor`; `BOUNDED_IDLE_FLOOR` overrides it, and a value that is not a number in 0 to 100 leaves the default. A reading at the floor counts as under it. Test `TestAR48_5_FloorMatchesRelay`, written from `DefaultIdleFloor` and never from a literal: with no `BOUNDED_IDLE_FLOOR`, an idle share of `DefaultIdleFloor` x 100 waits and stops `load`, and one point above it runs the command; with `BOUNDED_IDLE_FLOOR=80`, an idle share of 70 waits. P2's sweep reads 0.50; that change waits for the owner's word and will move both constants in its own change. No other test depends on the default: use 70 and 10, on the same sides of 25 and of 50.

6. **The fix round** (review round 1: two blocking, the rest small). `now` is taken after the load read, as on `main`; at or past the limit bounded refuses before any read; else the reader's timeout is the smaller of 5 s and the time left, and a timeout is unreadable. The waiting line's seconds come from that `now`, so with no reading stderr is `main`'s byte for byte, a hung `top` included. A read's time comes out of that poll's sleep, so the load is sampled at `main`'s cadence. The not-confirmed line never says the command runs: the wall may still stop it. A signal during a read kills the reader's child before bounded exits. `BOUNDED_IDLE_FLOOR` takes any number `float` reads in 0 to 100 (`+50`, `5e1`). `Makefile:15`'s help names the rule. Tests named `TestAR48Fix_` (the acceptance line still reads 5): a slow `top` under a short wall; a hung `top` at a short limit; a load that dips during a read; `10,60,10,60,10` and `10,60,x,60,10` never close; a TERM during a read leaves no `top`.

7. **The hardening round** (the verify's one regression). `top` stays in bounded's process group, so a kill of the group takes it; the timeout and the handler end it by pid. A poll that is not positive is the default. Two `TestAR48Fix_` tests.

After the acts: the docstring names the rule and both env names. `design/readme/source.md` and `README.md` say in their two `bounded` lines that the load holds a check only when the CPU confirms it: the same words in both, by hand (never `make readme`), then `make readme-stamp`.

## 3. Interfaces (signatures, not prose)

New in `scripts/bounded`: the env names `BOUNDED_IDLE` and `BOUNDED_IDLE_FLOOR`, one named constant for the default floor, one reader and its two pure parsers. Unchanged: the usage line, every option, every exit code, the four existing `BOUNDED_` names, the last stderr line, and the waiting and refusal lines when no idle share was read. The words of the new stderr lines are yours; each names the load, the idle share and the floor. `runBounded` passes no env: add your own helper in `ar48_test.go`, on a copy of the script by `copyIn`.

## 4. Findings that name this row

`scripts/bounded:12-13`, `:27-28`, `:79-83` (`env_float`), `:137-158` (`wait_for_load`), `:254-261` (the signal handler); `internal/night/ar17c_test.go:73-108` (`boundedPython`, `runBounded`); `internal/night/bound.go:631-639` (`DefaultIdleFloor`); `internal/effects/idle.go` (the same rule in Go); `Makefile:15-20`, `:47-48` (`check`, the README freshness check and its stamp); `README.md:90`, `:99`.

## 5. Forbidden reads

Other repos, the plugin repos' copies of `bounded` among them; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: the acceptance line's exit and last line, and `make doclint`'s. Bullets: each act's red test on `main`; how the test reaches the parsers; the words of the two new stderr lines; what was left. Last line: branch, head sha.
