# Packet AR47: the breaker's second key, the idle share

| item | value |
|---|---|
| from | `design/breaker-signal-r1.md` §2, §3 and §5 item 1; `design/relay-r1.md` §15, the **Breaker** bullet |
| pin | agent-relay `main` at e96be1c plus this packet's own commit; worktree `.claude/worktrees/ar47-breaker-idle-key`, branch `ar47-breaker-idle-key` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: fakes for the load, the idle share and the clock, in-process; no test runs `top` or loads the machine. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state |
| may touch | `internal/effects/load.go`, `internal/effects/idle.go`, `internal/effects/effects.go`, `internal/effects/*_test.go`, `internal/night/bound.go`, `internal/night/run.go`, `internal/night/*_test.go`, `internal/night/testdata/json-keys.golden`, `cmd/relay/main.go`, `design/relay-r1.md`, `design/tla/Night.tla` (its comment on the load only), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^TestAR47_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR47_"` reads 4 |
| budget | 80 turns, 150k context, `claude-opus-5-5`; one commit per act; `make check` in the foreground, then the last commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`design/breaker-signal-r1.md` §2: "loaded at k = the 1-minute load is above k x cores **and** the idle share of the CPU is under `IdleFloor`." "The idle share is read only after the load key has turned." "An unreadable idle share leaves the load key alone to decide, as today. The rule can only narrow when the breaker fires; it never fires where today's does not."

What was seen: on 2026-10-01 two plans of one row read load 52.7 and 40.23 on 16 cores with the CPU 59% to 73% idle, and each left 8 of 9 checks `unrun: loaded`. Twelve busy loops at background priority raised the load from 4.61 to 11.14 with the CPU 72% idle and a foreground job's time unchanged (§4, P1).

Today `Breaker.Wait` (`internal/night/bound.go:610`) opens above `Open` x cores on the load alone, and `boundShell` (`:413`) sets `loaded` above `Close` x cores on the load alone.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar47-breaker-idle-key` off `main`. The charter is `history/sittings/<the build date>-<n>-ar47.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is. **No existing test line changes**, and no key of an existing ledger line changes its name, type or meaning.

1. **The reader** (act 1). `effects.Idle` is `Idle() (share float64, err error)`, the idle fraction of all cores, 0 to 1. `effects.OSIdle` reads the last `CPU usage:` line of `top -l 2 -n 0 -s 1` on darwin and two reads of `/proc/stat`'s `cpu` line one second apart elsewhere. The parsing is a pure function. Text with no `% idle`, or a value outside 0 to 100, is an error. `Effects.Idle` is the field; `cmd/relay/main.go` sets it beside `Load`. Test `TestAR47_1_ParseIdle`, in `internal/effects` and so outside the acceptance line (the review and `make check` run it): two `CPU usage:` lines give the second's share; a line with no `% idle` is an error; two `/proc/stat` lines give the share of their difference.

2. **Both keys at `Open`** (act 2). `Breaker` gains the fields `Idle effects.Idle` and `IdleFloor float64`; `DefaultBreaker`'s floor is `night.DefaultIdleFloor`, 0.25, one named constant. `run.go` and `boundShell` copy `DefaultBreaker` and set `Idle` from the effects. `Wait`'s signature and its `note` type stay as they are. Above `Open` x cores `Wait` reads the idle share once: above the floor, the breaker does not open and `Wait` returns true; at or under the floor, or with `Idle` nil or failing, it opens as today. While open it closes when the load is under `Close` x cores, or when two polls in a row read the idle share above the floor. Test `TestAR47_2_OpenNeedsBothKeys`: load 70 on 16 cores with idle 0.70 never opens and waits nothing; with idle 0.10 it opens; open, one poll at idle 0.60 does not close it and a second does.

3. **Both keys for D4** (act 3). A line is loaded when, at the read before it or the read after it, the load is above `Close` x cores and the idle share is at or under the floor. The idle share is read only when that read's load is above `Close` x cores. Test `TestAR47_3_LoadedNeedsBothKeys`: a red baseline line at load 40 on 16 cores with idle 0.70 stays `red` and is stored in the memo; the same line with idle 0.10 is `unrun: loaded` and is not stored.

4. **The record** (act 4). A load above the threshold that the idle share did not confirm writes one `breaker` ledger line with `state: unconfirmed`, `load`, `cores` and `idle`, once per `Wait` and once per line's D4 read. An open or close line gains `idle` when it was read. `BreakerStats` gains `Unconfirmed`, JSON key `unconfirmed`, left out when zero, so `ar10_test.go:536` and the golden stand where the count is zero. Test `TestAR47_4_UnconfirmedLine`: the scenario of ruling 2's first case writes exactly one such line, and the plan's `breaker` value carries `unconfirmed: 1`.

5. **As today without it** (act 5). Test `TestAR47_5_NilIdleAsToday`: with `Idle` nil, and again with an `Idle` that returns an error, load 70 opens the breaker and a red line at load 40 is `unrun: loaded`.

6. **The fix round** (review round 1: one blocking, two small). A stop that lands during the idle read before a line is the owner's stop: `boundShell` reads `stopReason` again after that read, so the line never starts and the error is the stop's, never `budget` (`TestAR47Fix_StopDuringIdleRead`). The parsers refuse NaN, and a comma decimal is unreadable, not misread (`TestAR47Fix_ParseIdleRefuses`). The fix round's tests are named `TestAR47Fix_`, so the acceptance line still reads 4.

After the acts, §15's **Breaker** bullet states the rule, the floor and the new line, and `Night.tla`'s comment on the load names both keys.

## 3. Interfaces (signatures, not prose)

`effects.Idle`, `effects.OSIdle`, `Effects.Idle`, `Breaker.Idle`, `Breaker.IdleFloor`, `night.DefaultIdleFloor`, `BreakerStats.Unconfirmed` are new. `Breaker.Wait`, `effects.Load` and `breakerLine`'s existing keys are unchanged; how `idle` and the `unconfirmed` state reach `breakerLine` is yours to shape.

## 4. Findings that name this row

`internal/night/bound.go:333` (`boundShell` takes `DefaultBreaker`), `:380` (`Wait`), `:399-425` (the reads round a line and the memo store), `:528-540` (`noteBreaker`, `breakerLine`), `:573-650` (`Breaker`, `BreakerStats`, `Wait`); `internal/night/run.go:187` (the spawn's `Wait`); `internal/night/ar10_test.go:503`, `:507`, `:536` (callers and a byte-exact line that must stand); `cmd/relay/main.go:381` (`Load: effects.OSLoad{}`).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: each act's red test on `main`; how `idle` reaches the ledger line; any existing test you had to read to keep unchanged; what was left. Last line: branch, head sha.
