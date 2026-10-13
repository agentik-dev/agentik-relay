# ADV10: the second adversarial review of the relay, c4a0777..d3a4e76

Target: the 57 commits since the adv-6 merge. Two read-only Opus reviewers, two slices;
every repro ran in a temp repo or copy. The coordinator re-ran ADV10-2, 4 (by reading), 6, 8, 18, the signal grep, the hook probes.
`make e2e` (9/9) and `make tla` were green at e2e-10 (charter 2026-09-23-19).

## The ADV6 fixes as shipped

Holds: 3 (`gate.go:85-98`, `:534-539`), 4 (`effects/git.go:206`; copies, modes,
symlinks and submodules listed), 7 (`run.go:336-349`, E2E_6), 9 (`bound.go:138-239`),
11 (`spawn.go:66`), 13 (CLI cap `main.go:821`), 16, 19. Partial: 1 (`go run`, ADV10-6),
2 (plan untrapped, ADV10-5), 5 (the merge needs exit 0 at `gate.go:545`; the gate still
passes as-main, ADV10-16), 6 (ADV10-1, 3), 8 (ADV10-12), 10 (packet from the root, the
Makefile from the branch, ADV10-10), 12 (ADV10-4), 14 (ADV10-17). Grown: 15 (ADV10-8).
Open: 17, 18. No fix regressed.

## High

- **ADV10-1: a child can move `main` and skip the gate.** Nothing snapshots main's sha
  from stage to gate, and the hook allows `update-ref` (re-run: `{}`). Ran: in a linked
  worktree, `git update-ref refs/heads/main HEAD` moved the root's main. Fix: record main's sha at stage and fail the gate with `main-moved` if it
  changed. Deny `update-ref`, `branch -f/-M` and `-C <root>`.
- **ADV10-2: a tag named after the base hides every touched path.** `gate.go:123` diffs
  against the bare name `main`, and `effects/git.go:23` drops stderr. Ran:
  `git tag main HEAD` changed `diff --name-only main...HEAD` from `secret/a` to empty.
  `Outside=[]`, so the gate is green and the merge goes ahead. Fix: diff against the
  base's sha recorded at stage (`refs/heads/<base>^{commit}`), and treat diff stderr as
  unrun.
- **ADV10-3: a child can still push.** The only push rule is `^git push`
  (`hooks/guards.go:28`). Landed on a bare remote, the child's env:
  `git send-pack`, `$(echo git) push --no-verify`, and `git push --no-verify <url>`.
  `env -S`, `xargs`, `bash <<<` pass the hook too. The origin check
  (`gate.go:117`) reads local tracking refs, a URL push never updates. Fix: deny
  `send-pack`, `http-push`, `subtree push`, `$`-command words and shells fed from stdin.
  Compare `git ls-remote` against the stage snapshot.
- **ADV10-4: an acceptance line can still run the whole suite (the D1 shape).**
  `packet.go:179-184` refuses only the make target `check`. `Unbounded` accepted
  `make test` (`go test -race ./...`), `make check-unlocked` (the gate body, outside
  `scripts/bounded`), `make e2e` (3 min), `make vet`, `gmake check`, `$(echo make) check`,
  `m=make; $m check` and `echo 'make check' | sh`. doclint's `runs_suite` applies only
  when a verify row exists (`doclint:196`). Fix: refuse `test`, `vet`, `e2e`, `check*`,
  `gmake`, `$`-command words and stdin-fed shells. Run doclint's rule always.
- **ADV10-5: a killed `relay run` or `relay plan` leaves its measured line running,
  unrecorded.** `loadPlan` (`main.go:889`) measures before `night.Run` installs its trap
  (`night.go:291`). `plan` and `gate` never trap. Each line has its own process group
  (`shell.go:50`), so Ctrl-C misses it too. Ran: SIGTERM to `relay run --rows r1` after
  3 s; its `sleep 45` survived, no `groups.json`. Night.tla's `Recorded`
  includes `measuring` (`Night.tla:487`): the model claims what the code lacks. Fix:
  trap and `Watch` from `main`, before any measurement.
- **ADV10-6: the nested-run guard is bypassed by `go run` (ADV6-1 and ADV6-18).**
  `NestedRun` matches only `^relay (run|plan)` (`guards.go:42`). Ran: the hook returned `{}`
  for `env -u RELAY_ROW -u RELAY_MEASURE_DEPTH go run ./cmd/relay run --rows x`. With those
  variables unset, the binary plans and measures. Fix: match any command whose argv holds a
  `cmd/relay` path or a binary named `relay`. Put an ancestor pid file in the state dir,
  which the binary checks.
- **ADV10-7: the gate judges files the branch does not contain.** The clean check
  (`gate.go:108`) and the acceptance and verify lines (`:141`, `:261`) run in the child's
  own tree. Ran: after `git update-index --skip-worktree f`, status was clean, the tree
  held `faked` and `HEAD:f` held `real`; `info/exclude` hides files the same way. The merge
  re-runs verify only. Fix: run acceptance and verify in a fresh detached worktree at
  `g.Head`, as `Hand` does.
- **ADV10-8: 11 tracked `design/packets/rows-*.json` hold the owner's absolute home path
  in a public repo.** `.hygiene-allow:13` still admits `home`. Four commits since c4a0777
  added more. Ruled (run 10 ruling 8), carried by packets-34-relay.

## Medium

- **ADV10-9: a usage-limit or login failure stops the night only by accident.** Nothing
  detects it (`stream` ignores `rate_limit_event`). It reaches `stream-error` through
  `relay/gate.go:61` and `night.go:612`, but only if the CLI sets `is_error`. Under
  `--parallel` the other rows drain into the same limit. A gather error leaves the row
  unrun, and the night moves on (`night.go:613-623`). Ruling 1 (carried): add a
  `usage-limit`/`auth` stop before the gate that kills the running rows.
- **ADV10-10: a builder can neuter its own verify line.** `GuardedPaths` (`gate.go:39`)
  omits `Makefile` and `scripts/`. Ten packets list `Makefile` in may-touch. The gate and
  the merge both run the branch's `check-unlocked`. Fix: guard both, or run main's
  Makefile.
- **ADV10-11: the ADV6-6 pre-push fix disables the repo's own commit hooks.**
  `effects/git.go:261` points `core.hooksPath` at a directory holding only `pre-push`.
  Ran: a `pre-commit` that refused before is skipped after; this would hit example-app. Fix:
  link the common hooks in, and deny any change to `core.hooksPath`.
- **ADV10-12: a `setsid` daemon with closed stdio escapes, and its line reads green.**
  Ran: `perl … fork; setsid; exec sleep 77 >/dev/null 2>&1; echo ok` through `OSShell.Run`
  gave `exit=0`, and the sleep lived on. Fix: sweep by session id (D2 §9), from a pid
  snapshot taken before and after the line.
- **ADV10-13: the stale stage-lock takeover races.** `stage.go:161-173` reads, removes and
  then creates the lock. In a temp test, 6 concurrent takers left more than one holder in
  40 of 40 runs. Fix: `flock`.
- **ADV10-14: `scripts/bounded` takes the check lock from a live holder.** `STALE=900`
  equals `--wall`, and the load wait counts toward it. Ran: a lock owned by a live pid and
  901 s old was taken over. Fix: never take over a live pid.
- **ADV10-15: the guard hook fails open without `relay` on PATH** (`plugin/hooks/run:4`).
  Ran: `PATH=/usr/bin:/bin`, and `git push` was allowed. Fix: deny under `RELAY_ROW`, and
  pass the binary's absolute path in the child's env.
- **ADV10-16: same-as-main verify still passes the gate** (`gate.go:270`, `:212`). A
  branch-only row on a red main reads green. The merge path is safe.
- **ADV10-17: hygiene scans a NUL-bearing file for `home` only** (`scripts/hygiene:139`).
  Ran: a NUL-prefixed file holding a key-shaped string and a handle raised no hit. Fix:
  scan printable runs for every group.
- **ADV10-18: `relay resume` has no nested guard** (`main.go:1087`). The hook allowed it
  (`{}`). Fix: refuse under `Nested` and add `resume` to `NestedRun`.

## Low

- ADV10-19: row ids `HEAD`, `a..b`, `x.lock` and `main` pass `queue.go:341`. Fix: add
  `check-ref-format --branch`, and reserve the base.
- ADV10-20: the amend and rebase guards miss `reset --soft HEAD~1`, `commit-tree` and
  `-c pull.rebase=true pull`.
- ADV10-21: the live-claude probe's timeout kills only the process (`stage.go:69`). Each
  run of `TestAR13_5_LiveClaudeTimesOut` leaves an orphan `sleep 20`.
- ADV10-22: `TestAR10_0_NoTestOpensRealCheckout` scans only `main_test.go`;
  `gate_test.go:337` and `packet/ar10_test.go:61` read the real tree.
- ADV10-23: ADV6-17 is still open: `stream.go:202` reads unbounded lines.
- ADV10-24: `Packet.Verify` has no consumer (ruling 6, carried).

## TLA invariants with no test

Every other invariant maps to a Go test or an e2e case. None exercises: `TypeOK`
(model-only); the `measuring` half of `NoOrphanAfterExit` (Signal and Reap over a
measured line), which Go fails (ADV10-5); the reaped branch of
`EveryRunnableRowEventuallyGatedOrParked`; `NoNestedSpawn` by a `go run` spelling
(ADV10-6). The model has no escaped (`setsid`) process kind, so ADV10-12 is outside TLC.

## Verdict

**Not ready to carry example-app.** The spelled-out fork-storm cases are closed; three doors
remain: another spelling of the suite (4), `go run` (6), a killed plan's orphan (5). A
child alone reaches three new wrong greens (1, 2, 7) and can push (3). Fix first:
1 to 7, 9, 10; 11 to 15 and 18 in the same packet (11 matters: it has commit hooks).
8 is carried; 16, 17 and the lows wait.

## The three tests to add first

1. `TestADV10_GateBindsBase`: a child moves `main`, then tags `main`; each gate is red
   with a named reason and nothing merges.
2. `TestADV10_UnboundedSpellings`: every ADV10-4 spelling, refused by `packet.Unbounded`
   and doclint.
3. `TestADV10_SignalMidPlan`: SIGTERM to `relay plan --rows r` mid-baseline; no
   descendant alive, `groups.json` records the line.
