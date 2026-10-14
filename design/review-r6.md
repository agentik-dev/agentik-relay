# R6: the design-decisions review of agent-relay (runs 1 to 5, 2026-09-22 to 23)

Read: the defect brief, every packet's rulings and touch rows, the design of record, review-ar2, the log since 2026-09-22, the code the cycles run through. The charters' handover sections were not read (the sandbox refused the read): the charter-only gaps here are the ones the packets, the design and the log expose, plus one seen in passing (R6-27).

## 1. Decisions ledger

"record" = in `design/relay-r1.md`; "gap" = only in a packet, charter or script, with the section it belongs in.

- R6-1 D16 tmux optional, file windows the tested mode (A9): record §13, §14.
- R6-2 D17 own strict YAML parser (A4): record §3.
- R6-3 D18 the ledger never enters a repo (A5): record §5.
- R6-4 D19 sitting failures stop the night, gate reds move on (A3, A8): record §1.
- R6-5 D20 the driver picks `--session-id` (A7): record §7.
- R6-6 D21 exit codes 0/1/2/3 (A3): record §9.
- R6-7 D22 merge is no-commit, verify, commit, else abort (A6): record §6.
- R6-8 D23 user config holds ntfy and tmux, the repo file queue and merge: record §9.
- R6-9 pre-allowed working tools in the spawn (owner 10:35): record §7.
- R6-10 `relay hook` exits 0 always; deny push, amend, rebase; Stop blocks 3 times (AR1): record §10.
- R6-11 globs match per segment, never `**` (A1): record §2, §12.
- R6-12 a dependency is satisfied by main's ancestry, never the row's base (AR4-1): record §3.
- R6-13 doctor probes `claude --max-turns 1 --version` (AR4-2): record §9.
- R6-14 every touch set admits the charter and NEXT (AR4-3): record §6.
- R6-15 an unrun gate is red `gate-unrun` (AR4-4): record §1.
- R6-16 plan measures verify once; the merge takes green or same-as-main (AR4-5): record §6.
- R6-17 the sitting line a superset of the stand-in's; `gate` an object; snake_case (AR4-7, 8): record §5, §9.
- R6-18 `XDG_STATE_HOME` the one state root; a `TestMain` per package (AR4-9): record §5.
- R6-19 the owner's kill escalates; a lost spawn line is `ledger-error` (AR5-5): record §7.
- R6-20 the canonical scripts pinned by sha256 in the Makefile; pluginlint born (AR6), retired (AR8); doclint once (AR9-3): gap, §12.
- R6-21 the README sidecar; `make readme` never in check (AR6-3): gap, §12.
- R6-22 the gate runs verify after the acceptance lines (AR7-13): record §6.
- R6-23 the id grammar, case-folded uniqueness, a local packet path (AR7-9, 10): gap, §3.
- R6-24 gofmt in check (AR7-17): the Makefile; conventions §5's, not this record.
- R6-25 landed rows are dropped, never parked; the rows files are the record (AR9-1): gap, §3.
- R6-26 AR9's `AR9_NESTED` self-guard: superseded by `RELAY_MEASURE_DEPTH` (AR10-0).
- R6-27 `tmux: required`, a session lost while draining is treated as `auto` (charter 2026-09-22-10): gap, §9.
- R6-28 AR2-13 refused: a plan-time satisfied edge outranks a red tonight: gap, §3 (only in review-ar2).
- R6-29 AR10 acts 0 to 9: record §15.
- R6-30 `stalled` is red; `load`, `invariant`, `orphans` are failed (AR10): record §1, §15.
- R6-31 §14's D16 to D23 "recommended, unruled": settled by the standing word of 16:30; gap, retitle §14.

The first thirteen charters escalated 59 decisions (their closing commits); after the standing word, one fork per packet.

## 2. Reversals and tensions

- The acceptance grammar. Every packet before AR10 carries `make check` and `go test -race ./...` as acceptance lines (A1's charter asked for `-race` in check; A3 to AR8 copied it into acceptance). AR10's `packet.Parse` refuses them and grandfathers the old. Only this binary knows the rule; the other repos' packets pass the canonical doclint. Yield: conventions §5 and the canonical doclint carry it; the binary keeps its copy.
- Verify's home: AR6 added pluginlint, AR8 retired it, AR9 runs doclint once. Settled; §12 says none of it.
- `stalled` red (moves on) beside `load`, `invariant`, `orphans` failed (stops): a stall can be the driver's as much as the packet's. Keep, with the stand-in's rule: a second stall in one night stops it.
- §14 still says "for the owner" over eight settled items (R6-31).

## 3. Fork-storm class (the cycle; the bound today; the fix)

- C1 plan measures a line that runs the built binary (`TestAR10_0_NestedPlanMeasuresOnce`): the inner plan sees `RELAY_MEASURE_DEPTH` from `OSShell` and measures nothing; the outer is un-nested by a filtered env over a fixture queue. Bound: the depth var and the fixture. Tested.
- C2 the gate reruns the same test in the worktree: the same bound.
- C3 the verify line runs the whole suite: C1 plus the fake-claude nights (temp repos, no hooks, no relay). Bound: fakes; the memo makes it once per head.
- C4 **a relay child runs `relay run`.** The child's env carries `RELAY_MEASURE_DEPTH=1`; a nested plan measures nothing but stays valid (`Plan.Exit` reads `Valid` only), and neither `runVerb` nor `night.Run` reads `Nested`; the child's toplevel is its worktree, so the other rows stage under it and spawn grandchildren with the same env, each able to repeat it. Bound: NONE in the binary; the wall's prose and each level's `--max-turns` and `--max-budget-usd`; the branch-exists refusal covers the child's own row only. Fix: `run` refuses a nested plan before any stage (exit 2, `stop: nested`), `relay.Invariants` gains `never-nested-run`, the PreToolUse guard denies `relay run` and a bare `relay plan` when `RELAY_ROW` is set.
- C5 `/agent-relay:plan` runs a whole-queue plan: bounded by scope, the memo, 600 s and the breaker. No cycle.
- C6 `make readme` starts `claude -p`: 8 turns, never in check, no edge back. `packet.Parse` does not refuse it as an acceptance line; refuse any line that starts `claude`.
- C7 `make check` runs `claude plugin validate`: no session, no hooks, no edge back.
- C8 the Stop hook: bounded by `maxBlocks`, `stop_hook_active` and Claude Code's 8-block cap (hooks-guide, 2026-10-14).
- C9 the stand-in measuring this repo: C3 under example-mac's depth var; no test opens the real queue (tested).

## 4. Timing and edge conditions (the invariant; tested?)

- `LockRoot` is a process mutex: a hand plan or gate beside the night measures in the same root, or reads it mid-merge. One measurement per root on this machine. No test. Fix: `flock` on `<state>/locks/root-<sha1>`.
- The same-as-main anchor carries no head: `Plan.Verify` measured at M0 anchors a merge onto M1 after an earlier merge tonight; a red equal to M0's lands on a green M1. The anchor's head equals the base head. No test. Fix: `Check` gains `head`; else the memo at main's head; else green only.
- A stage lock is `O_EXCL` with a pid never checked: a crashed relay leaves the row refused until a hand removes lock and worktree. A lock with a dead pid is not held. No test. Fix: reclaim on ESRCH with a ledger line; `relay clean --stale`.
- `LiveClaude` runs `lsof` with no timeout (Stage, Hand); under load it takes minutes. Every probe returns within a bound. Fix: 30 s, then "unknown", a refusal.
- `Breaker.Wait` takes `context.Background()`: a `stop` or `kill` during a wait lands up to 10 minutes late. Fix: the night's context.
- The memo has no age: a green at head H stands across a `go` or `claude` upgrade. Fix: `ts` over 24 h is a miss.
- The gate's reruns and `--parallel N` bypass the breaker: N suites at once. Fix: the breaker ahead of every rerun.
- doclint and hygiene run through the pyenv shim (`env python3`) inside the verify line; D5 saw 60 s per shell on the rehash lock. The verify line waits on no interpreter lock. Fix (conventions, all four repos): the Makefile resolves the interpreter once.
- The wall is never zero; the watchdog kills at 45 minutes of no HEAD, mtime or CPU: tested (`TestAR10_6_*`, `TestWallClockKill`).

## 5. What a TLA+ model should cover

PlusCal, TLC over 2 rows, 2 slots, 3 heads, load in {low, high}, depth in {0, 1}: the night machine with the breaker, the depth guard, the process group, the stage lock, the memo and the anchor head. Properties: no shell line at depth 1 (C4 fails it today); one measurement per root across processes (fails today); the anchor head equals the base head (fails today); one sitting per tree; every child ends by wall or stall under a fair clock; every reachable state reaches `stopped:*`, draining and breaker waits included; a kill before a start leaves no live process. One packet: `design/tla/night.tla` and its `.cfg`, before the first example-app night.

## 6. Decisions for the owner

1. Refuse `relay run` when nested, in the binary and the child's guard (C4). Recommend yes. Consumer: the next relay packet.
2. The targeted-acceptance grammar into conventions §5 and the canonical doclint. Recommend yes. Consumer: the session's conventions row, then a re-sync here.
3. One packet for the anchor head, the cross-process root lock and stale-lock reclaim (§4's three with no test). Recommend yes, before example-app. Consumer: the next relay packet.
4. The TLA+ model of §5. Recommend a small model now, not after the first night. Consumer: the owner's word, one packet.
5. The pyenv shim out of every `make check`. Recommend the resolved interpreter in the Makefile. Consumer: conventions §6, then the re-sync.
