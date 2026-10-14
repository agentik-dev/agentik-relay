# agent-relay design: the breakers' load signal (r1)

**Status:** design r1, 2026-10-01, written before code and revised once after its one refuter round (§7), which did not block. It amends relay-r1 §15's **Breaker** and **`check` bounded** paragraphs and the D4 rule; no entry of `DECISIONS.md` covers the signal. Both proofs ran (§4). AR47 is merged at 4b980d7 with the floor at 0.25; P2's sweep says 0.50, a one-constant change that waits for the owner's word.

## 1. The problem

Two breakers hold work back while the machine is loaded, and both read one number: the 1-minute load average against the cores.

- `night.Breaker` (`internal/night/bound.go`, through `effects.OSLoad`): above 4 x cores it opens and waits up to 10 minutes, then leaves a baseline line `unrun: load` or stops the night `stopped:load`. D4: a red line measured above 2 x cores is `unrun: loaded` and anchors no same-as-main.
- `scripts/bounded --load 4` (every `make check`, here and in the plugin repos): above 4 x cores it polls for up to 600 s, then exits 2.

On this Mac the load average does not follow the CPU. It counts every runnable thread, and the scheduler keeps background-priority threads on the 4 efficiency cores of 16, where they queue while the 12 performance cores stand idle.

| seen | load (1 min) | idle | effect |
|---|---|---|---|
| row gw's run, night 2026-10-01-1, plan baseline (00:09) | 40.23 on the `plan` line; 31 to 72 over the hour | 59% to 73% (`mediaanalysisd` at 156% to 324%) | 8 of 9 checks `unrun: loaded` |
| row gw's hand plan five minutes earlier (07:04:45Z, named night 2026-09-30-10) | 52.7 | not read | 8 of 9 checks `unrun: loaded` |
| 2026-10-01 07:24, no heavy work | 10.49 | 90.3% | none |

At 72 the breaker is open (above 64) and `make check` is held, with more than nine cores idle. gw went green all the same, because its red baselines were acceptance lines that are red on main by design. A red verify line under the same load would have lost its anchor.

## 2. The decision

A high load reading counts only when the CPU confirms it. Each place that compares the load with a multiple of the cores gains a second key:

> loaded at k = the 1-minute load is above k x cores **and** the idle share of the CPU is under `IdleFloor`.

- **The idle share** is the idle fraction of all cores over a one-second sample. On darwin it is the second `CPU usage:` line of `top -l 2 -n 0 -s 1`: the second line is the change over the one-second wait, and the first line's window is not defined. On Linux it is two reads of the `cpu` line of `/proc/stat`, one second apart.
- **`IdleFloor` is 25% in the code** (`night.DefaultIdleFloor`); P2's sweep (§4, ran 2026-10-01) reads 0.50, a one-constant change that waits for the owner's word. Background work that fills the 4 efficiency cores leaves 75% idle at the least; 32 or more runnable threads at normal priority leave none. The floor sits between the two, and a reading at the floor counts as under it.
- **The idle share is read only after the load key has turned.** A quiet machine pays nothing; a loaded one pays 1.4 s per read (measured twice).
- **An unreadable idle share leaves the load key alone to decide**, as today. The rule can only narrow when the breaker fires; it never fires where today's does not.

In `night.Breaker.Wait`: the breaker opens when both keys are turned at `Open`. While open, each poll reads both, and it closes when the load is under `Close` x cores, or when two polls in a row read the idle share above the floor. One quiet second does not close it. D4: a line is loaded when both keys are turned at `Close`, at the read before the line or at the read after it. The read after a line samples the second after it ended, so it measures the other work on the machine and not the line's own threads, which the load average still carries.

In `scripts/bounded`: `wait_for_load` applies the same rule with the same reader, in the standard library.

## 3. The seam and the record

- `effects.Idle` is a new interface, `Idle() (share float64, err error)`, with `effects.OSIdle` and a field `Effects.Idle`. Nil means the load key alone decides.
- The reader reaches the breaker as fields of the `Breaker` value (`Idle`, `IdleFloor`), never as a parameter of `Wait` and never through its `note` callback: `ar10_test.go:503` and `:507` call `DefaultBreaker.Wait(ctx, load, clock, note)` with `note func(bool, float64, int)`, and `run.go:187` calls the package value. `run.go` and `boundShell` copy `DefaultBreaker` and set `Idle` from the effects. With that, no existing test line changes.
- The `breaker` ledger line gains `idle` whenever it was read. A load reading the CPU did not confirm writes one `breaker` line with `state: unconfirmed`, `load`, `cores` and `idle`, once per wait, so a night shows how often the old rule would have fired.
- `BreakerStats` gains `unconfirmed`, a count, beside `opens`. The key is left out when it is zero: `ar10_test.go:536` matches `"breaker":{"opens":1,"wait_s":60,"load":70}` byte for byte, and `testdata/json-keys.golden` freezes the plan's `breaker` keys, so both stand where the count is zero.
- `scripts/bounded` reads `BOUNDED_IDLE` (a fake idle share, as `BOUNDED_LOADAVG` fakes the load) and `BOUNDED_IDLE_FLOOR` (25). Its waiting line on stderr names both numbers.
- `Night.tla` models an unrun line and needs no change but a comment (`:15` names the 1-minute load).

## 4. The proof

**P1, ran 2026-10-01 07:26 (a night was live, so the scale was kept under 2 x cores).** Twelve busy loops under `taskpolicy -b` for 75 s; a fixed foreground job (`sum(range(20_000_000))`, best of three) timed at each sample.

| sample | load (1 min) | idle | foreground job |
|---|---|---|---|
| before | 4.61 | 88.7% | 0.154 s |
| +25 s | 7.03 | 72.9% | 0.139 s |
| +50 s | 9.07 | 72.9% | 0.143 s |
| +75 s | 11.14 | 72.3% | 0.147 s |

The load rose toward one per background thread, the idle share stayed above 70%, and the foreground job kept its time. Under the rule this reading is not loaded at any k.

**P2 as planned (it ran 2026-10-01, below), with no `relay run` or `relay gate` in `ps`.** It has two parts. First, forty busy loops at normal priority for 75 s: the rule holds if the idle share reads under 25% while the load climbs past 32. If the idle share stays above 25% under real saturation, this design is revised before any packet. Second, a sweep that sets the floor: over background work that holds the load above 32, add 4, 8 and 12 loops at normal priority, and at each step read the idle share and time a 16-wide job. The floor is the idle share at the first step where the 16-wide job slows by more than a tenth.

**P2, ran 2026-10-01 13:10 in a compact form, with no relay verb and no check running.** Forty-eight loops at background priority for 100 s to carry the load past 32, then loops at normal priority added in steps. At each step: the idle share, and a 16-wide job (sixteen processes, `sum(range(12_000_000))` each), one run.

| background | normal | load (1 min) | idle | 16-wide job |
|---|---|---|---|---|
| 0 | 0 | 8.97 | 92.0% | 0.20 s |
| 48 | 0 | 44.25 | 74.0% | 0.19 s |
| 48 | 4 | 47.42 | 49.4% | 0.24 s |
| 48 | 8 | 48.27 | 24.4% | 0.30 s |
| 48 | 12 | 49.61 | 0.0% | 0.35 s |
| 48 | 16 | 51.64 | 0.0% | 0.33 s |
| 48 | 40 | 56.79 | 0.0% | 0.46 s |

The first part holds: under forty loops at normal priority the idle share reads 0.0% with the load past 32. Background work alone, at load 44, leaves 74% idle and the job's time unchanged. The sweep does not support 25%: the job slowed by a quarter at the first step, four normal loops and 49.4% idle, and by more than half at 24.4% idle. By this section's rule the floor is 0.50. The job is short and each step is one run, so the number is coarse, but the trend is steady. The incident's readings, 59% to 73% idle, stay above 0.50. `night.DefaultIdleFloor` is still 0.25 in the code: the change to 0.50 is one constant, the line of `ar47_test.go` that pins it, and §15's number, and it waits for the owner's word.

The refuter's reading in §7 already stands for the first part: a real `go test` at load 17.30 read 2.17% idle. So AR47 may build before P2; the floor is one named constant, `night.DefaultIdleFloor`, which the sweep then sets.

## 5. The cut into packets

1. **AR47, the relay's breaker.** `effects.Idle` and `OSIdle`, both keys in `Breaker.Wait` and in the D4 rule, the `unconfirmed` line and count, §15's text. Acceptance by named tests: a high load with a high idle share neither opens nor marks a red line loaded; a high load with a low idle share opens and marks as today; a nil or failing `Idle` behaves as today.
2. **AR48, `scripts/bounded`.** The same rule and the two env names, and the README's line. This repo's one test of `bounded` covers the lock only (`TestAR17C_3_BoundedKeepsLiveLock`, with the helpers `boundedPython` and `runBounded` in `internal/night/ar17c_test.go`), and a packet's acceptance admits only `go test` lines, so AR48 first adds Go tests that run the script under `BOUNDED_LOADAVG` and `BOUNDED_IDLE`; those tests are its acceptance. A faked load with no `BOUNDED_IDLE` reads no idle share, so the plugin repos' tests that fake the load stand when they take the copy.
3. **Follow-ups, outside any row.** A row writes only its own repo, so the copies elsewhere are separate work: one commit in each plugin repo that carries a copy (claude-code-session, claude-code-decisions and claude-code-respeak share one copy today, which already differs from this repo's), and the conventions document's section on `bounded`, which lives in claude-code-session.

AR47 installs with the relay; AR48 reaches a repo at its next `make check`.

## 6. Open risks

- **The load key still gates, and it misses real saturation.** During another session's `go test`, the machine read load 17.30 at 2.17% idle: 1.08 x cores, not loaded under today's rule or this one. Across five ledgers the breaker has never opened: 0 `breaker` lines, 0 opens in 166 plans, highest load 52.7. This design only narrows; it does not make the breaker see a saturated CPU at a low load. A rule that reads the idle share first would, at 1.4 s for every measured line, and it needs its own evidence (NICE_TO_HAVE).
- The code's floor, 0.25, is a judgment; P2's sweep reads 0.50, coarse (one run a step), and the change waits for the owner's word. Background work above load 32 with eight normal threads leaves exactly 25% idle, and a 16-wide line then has four performance cores free.
- A one-second idle sample is an instant, and the load average is a minute. The close rule asks for two polls in a row; the open rule and D4 still act on one sample.
- A spawn has no D4 behind it: a child started on a machine the rule calls not loaded just runs slowly.
- `top` prints a localised line. The reader matches `% idle` and treats anything else as unreadable.
- The floor is one number for every machine, and Linux counts threads in uninterruptible I/O in its load average. Nothing here measured either.

## 7. The refuter round (2026-10-01, 07:29 to 07:37)

One refuter, read-only, at `c1bce88`. Verdict: does not block. No decision in force covers the signal, §1's account of today's code holds (`bound.go:413`, `:580`, `:627`, `:648`; `scripts/bounded:148-153`), and D4's ordering holds: `shell.go` sweeps the line's group and session before `Run` returns, and `bound.go:408` reads after. Four findings, all taken:

1. The load key misses real saturation (load 17.30 at 2.17% idle). Now §6's first risk, and filed.
2. "No existing test line changes" needed the constraints now in §3: the reader as fields of `Breaker`, and `unconfirmed` left out at zero.
3. Three facts were wrong and are corrected: `top`'s first line is not the average since boot; the earlier gw plan at load 52.7 also left 8 of 9 checks `unrun: loaded`; a read costs 1.4 s, not one.
4. AR48 had no harness in this repo and its plugin commits cannot ride a row. §5 now adds the Go test and names the follow-ups.

From its unconfirmed observations: the close rule now asks for two polls in a row (one quiet second closed the breaker at load 70 and restarted its ten-minute limit); a reading at the floor counts as under it; P2 gained the sweep that sets the floor.
