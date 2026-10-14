# agent-relay design: one gate verify at a time per root (r1)

**Status:** design r1, 2026-10-14, written before code and revised once after its one refuter round (§7), which passed with eight notes; the revision takes six of them. It amends relay-r1 §6 (the gate's verify) and §8 (concurrency). The settled entry 10 (`DECISIONS.md:37`, the verify limit) governs the limit, unchanged. The finding is `NICE_TO_HAVE.md`'s "Side-by-side verifies outlast the fixed cut".

## 1. The problem

Under `--parallel N` each row gates in its own goroutine (`internal/night/night.go:775`, `internal/night/run.go:496`). The gate runs the repo's verify line after the acceptance lines (`internal/gate/gate.go:486`) with no lock; the line's clock starts at `gate.go:926`, and `effects.Cut` (`internal/effects/effects.go:170`) reads it against `verify_seconds` (relay-r1 §6, §9). Gates that close together run the whole suite side by side, each slower than alone, while the limit and `doctor --verify`'s suggestion (twice one run) assume one run alone. On night 2026-10-13-7 the consumer's po2fix and us3afix verifies started at 07:41:16Z and 07:41:18Z and both were cut at 1800 s at 08:11:25Z; hand gates later went green in 16 to 17 minutes. The cut read the crowd, not the line.

## 2. The decision

1. **The gate lock.** A gate's verify line takes its root's gate lock just before its clock starts (`gate.go:926`, after the baseline's anchor) and releases it when the line returns: one gate verify at a time per root. The lock is process-wide, one slot per cleaned root; the account lock already makes the night the only process that gates (`relay gate` takes it, relay-r1 §8).
2. **The wait is not the line's.** The clock starts once the lock is held, as the merge's does after the root's lock (`gate.go:1584`, `:987`); the limit and the cut are entry 10's.
3. **The merge and the measure keep the root's lock** (`internal/night/bound.go:274`) and never take the gate lock, so the loop, which runs merges (`night.go:1036`, `:1317`), never waits behind a gate. At most two full runs per root overlap: a gate's and a merge's or measure's.
4. **A stop drains through the wait; the owner's kill ends it.** On `stop` (the night drains, `internal/relay/types.go:230`) or an account stop (`held.stopFor`) a waiting verify keeps its place and runs under its full limit: its sitting is done, a gate spends no tokens, and entries 11 and 12 pause or move only rows that do. After `stop` the row closes green or red. An account stop lands it as a row killed in its gate does today: paused by `holdRunning` (`night.go:1120-1133`: Exited, Limited, Paused) it keeps its verdict (AR70 ruling 3), stopped by `accountKill` it fails with the account's reason (`night.go:1302`). Only the owner's kill (`kill <row>`, `kill`, a signal: `held.kill`, `night.go:218`) closes the row's kill channel. The wait checks it before trying the lock, then selects on both, so a kill wins over a free lock: a row killed in its sitting is still gated (`run.go:494`) with its channel closed, and `select` picks a ready case at random. The line never runs: the verify reads `unrun`, last `killed waiting for the gate lock after <n> s`, and the row fails as the kill makes it (`owner-kill`, as a kill before the spawn does, `run.go:207`), its tree and branch kept for `relay gate <row>`. A channel, not the context: a stop (`owner-stop`, `night.go:1412`) and an account stop cancel the context too, and neither may end the wait.

**Rejected.** A cut scaled by the verifies in flight: the slowdown is not the count (§4: two others cost 2.1x, one other 1.4x), a hang reads k times later, the count changes mid-run while the shell's deadline is fixed at its start (`internal/effects/shell.go:101`), and a line is cut past the limit entry 10 lets the repo set and past §9's 3600 s bound. The root's lock for the gate's verify: a merge in the loop would wait behind a gate, writing no status and reading no control, up to one more limit. `--parallel 1`: it serialises the sittings, the part that spends tokens, to fix the part that does not.

## 3. The seam and the record

- `internal/night`: `GateLock(root, holder, killed, waiting)` returns the unlock and the seconds waited only once it holds the lock, so when the lock is held at the try it calls `waiting(behind, since)` before blocking. `held` gains the kill channel, closed once in `kill`; the row's `Gate` func receives it.
- `internal/gate`: `Verify.Killed` (nil for a hand gate: never closed). `relay.Check.Wait`, seconds, `json:"-"` as `Cut` is.
- The ledger: `waiting` writes one `verify-wait` line (`row`, `night`, `behind`: the holder's row); the verify's `gate` line gains `wait_s`, absent at 0. `cut` keeps naming the limit the line ran under.
- `status.json`: each row gains `wait`, `verify behind <row> since <hh:mm:ss>Z` from `waiting`'s mark until `GateLock` returns, else `""`; the text line ends `verify waits behind <row> <m>m`. The row's `state` stays `running`, the night's `running` or `draining`. The key re-freezes `internal/night/testdata/json-keys.golden`.
- relay-r1 §6 and §8 gain a sentence each.

## 4. The proof

Read from the account's ledger on 2026-10-14 (18 day files since 2026-09-22). A gate's span runs from its row's `exit` line to its result line (a gate writes its lines at its end; a verify's own start is only in a kept log's name).

**P1, the cuts.** Of 289 verify `gate` lines, 3 carry `cut`, all 1800 s, all the consumer's: po2fix and us3afix (§1) and usmark (night 2026-10-14-1, 11:06:04Z to 11:36:25Z). Each ran beside two other gates: po1bfix (green, a 1811 s span), and po3fix (1827 s) and usdoor (1579 s). Before entry 10, 3 exit-124 verifies were cut at 900 s, the two in nights alone: entry 10's case.

**P2, by gates in flight:** 71 night gates since entry 10 (2026-10-10), all the consumer's; medians over spans of 600 s or more.

| others in flight | gates | median span | longest | cut |
|---|---|---|---|---|
| none | 56 | 849.5 s | 1054 s | 0 |
| one | 6 | 1199 s | 1362 s | 0 |
| two | 9 | 1827 s | 1855 s | 3 |

"One" holds wfdue (31 s, no verify line). "None" ignores the sittings' own test runs: po3fixb (night 2026-10-14-2, closed 14:00:52Z, after this census) took 1455 s with no other gate, beside a row's `go test`. doctor's 1740 s for an 850 s run holds with no other gate and fails three side by side.

**P3, the cost.** Under the lock, night 2026-10-13-7's three verifies run in turn at 14 to 17 minutes each and all close by 08:24Z to 08:32Z, none cut. Side by side the night closed one green at 08:15:08Z and two red, green only by hand at 09:07:02Z and 09:23:25Z. When nothing is cut, the lock closes the last of three 9 to 17 minutes later.

## 5. The cut into packets

Numbers are the next free at writing.

1. **AR78, the gate lock.** §2 and §3 but `status.json`. Named tests in `internal/gate` and `internal/night`, with a fake shell that blocks: two rows of one root under `--parallel 2`, the second's verify starts after the first's ends; the fake line exits 124 at its deadline (as `effects.Cut` needs, §1), and a line that waits 1.2 of its limit and runs 0.6 is green with `wait_s`, which a clock started before the wait would cut; two roots never wait; the merge's verify takes no gate lock; `kill <row>` while waiting ends the wait within a second, no line started, verify `unrun`, the row `owner-kill`; `stop` or an account stop while waiting runs the line, the row landing per §2.4; a row killed in its sitting never runs the line, lock free; the `verify-wait` line names the holder; a hand gate (nil channel) waits and runs.
2. **AR79, the wait in status.** §3's `wait` key and text line; a row blocked in `GateLock` reads `wait` from `waiting`'s mark, any other `""`. Its may-touch names `json-keys.golden` (§3): only its absence barred AR59 (`NICE_TO_HAVE.md:369`). After AR78.

## 6. Open risks

- **Two repos in one night** still share the machine: the lock is per root.
- **A kill does not end a running verify** (its shell keeps its own deadline, `shell.go:101`): a waiter waits for its end or cut. Unchanged.
- **A hang still reads as a cut** with no goroutine dump; the finding's SIGQUIT note stays in `NICE_TO_HAVE.md`.
- **A merge or a measure beside a gate** makes two runs at once (P2's one other, 1.4x), inside doctor's suggestion.
- **A stop overshoots more.** Under `--parallel 3` the third gate waits up to two limits, and `stop`, `--until` and an account stop drain through it: a night runs past its stop by up to N limits, not one.

## 7. The refuter round (2026-10-14)

One adversary, read-only, at 579fce4: PASS, 0 blocking, 8 notes. Taken in the revision: note 1 (the census holds; the 59% line gone: §4), note 3 (the overshoot grows to N limits: §6), notes 4 and 5 (a kill wins over a free lock; an account stop's pause holds a waiting verify: §2.4), note 6 (AR78's fake line exits 124, its wait past the limit: §5), note 7 (`waiting` marks a wait before it blocks; AR79 may touch the golden: §3, §5). Notes 2 and 8 confirm the record: no deadlock (the anchor's root lock is released at `gate.go:922`, before the gate lock at `:926`), and the merge verify's limit stands.
