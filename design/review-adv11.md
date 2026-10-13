# ADV11: the third adversarial review of the relay, 6e2cd19..dce44c8

Target: the AR17 to AR19 code since the adv-10 merge (gate, guards, effects, night,
stream, packet, scripts). Two read-only Opus reviewers; every repro ran in an
archive copy with throwaway tests, or through the binary.s `hook PreToolUse`.
The coordinator re-ran ADV11-1 to 5 and read ADV11-12's matcher.

## The ADV10 fixes as shipped

Holds: 2 (`gate.go:162`, `gitStrict`), 5 (SIGTERM mid-baseline, no `sleep 45` left),
8 in the tree, 9 (the stop; see ADV11-12), 13 (`flock`), 14, 15 (`no-binary` deny), 19, 24.
Partial: 1 (ADV11-1, 2), 4 (ADV11-10), 6 and 18 (ADV11-9), 7 (ADV11-3), 10 (ADV11-4),
11 (ADV11-14), 17 (ADV11-15), 22, 23 (ADV11-17). Open: 3 (ADV11-6, 7), 12 (ADV11-8),
16 (ADV11-13). No fix regressed. Every AR17 fix is right about its spelling,
but the gate still reads state the child can write.

## High

- **ADV11-1: one forged ledger line undoes main-moved.** `expectedBase` (`gate.go:259-276`)
  takes any `merge`/`merged` line. The ledger is a file in the user's state dir, and the
  child can write it (no guard names it). Ran: `update-ref refs/heads/main <evil>` plus an
  appended `{"event":"merge","result":"merged","sha":"<evil>"}` gave `Run = green, main_moved=false`
  and a merge with `evil.txt` on main. Fix: the relay keeps the shas of its own merges in
  memory, keyed by root, and never reads them back from the ledger.
- **ADV11-2: a row whose base is not main never checks main.** `gate.go:171-176` compares
  only the base, and `MergeIfGreen` compares the root with `g.Main`, which is read at gate
  time. Ran: base `b0`, the child moves main: green, merged, `evil.txt` on main. Fix: record
  main's sha at stage for every row and compare it too.
- **ADV11-3: the gate's fresh tree runs the child's hooks.** `Fresh` runs
  `git -C <child tree> worktree add` (`effects/git.go:200`), and that fires the child's
  `post-checkout` in the relay's env. Ran: that hook
  repaired the file the branch broke, and the verify line read green. Fix: build the tree from the root under `-c core.hooksPath=/dev/null` with a
  sanitized config (no filters, no fsmonitor).
- **ADV11-4: `GNUmakefile` outranks the harness overlay.** `HarnessPaths` and
  `GuardedPaths` (`gate.go:40,47`) name only `Makefile`. Ran: the branch's `GNUmakefile`
  made `make check` exit 0, `outside=[]`, and it merged a file main's check rejects. Fix: run
  the verify as `make -f <main's Makefile copied outside the tree>`, or guard every make
  spelling and its includes.
- **ADV11-5: the root's merge commit runs child-written hooks.** `Commit` (`git.go:295`)
  fires the common dir's `pre-commit`, which the child can write. Ran: the hook added
  `evil.txt` and rewrote `a.txt`. The merge landed with "verify green" in its message, and
  the hook is still in the owner's repo afterwards. Fix: commit under
  `core.hooksPath=/dev/null` and `--no-verify`, then assert the committed tree is the
  verified tree.
- **ADV11-6: the verify line can push, and the gate reads green.** The `ls-remote`
  comparison runs before the lines (`gate.go:137-157`), and the lines run the branch's code
  in the relay's env. Ran: the branch's `t.sh` pushed `HEAD:main` and the gate was green,
  with the remote's main moved. Fix: run the lines under the child's pushurl deny and
  repeat the comparison after the lines and after `mergedVerify`.
- **ADV11-7: the origin check follows a URL the child can repoint.** Ran: a push by URL,
  then `git config remote.origin.url <mirror>`. The gate was green with `origin_same=true`,
  the real remote was moved, and the hijacked URL stayed in the root's config. The ADV10-3
  shapes are still allowed: `bash x.sh`, `python3 x.py`, `make push`, `watch`, `script`,
  `flock`, `fish -c`, `git submodule foreach`, `bisect run`, `difftool -x`,
  `remote add` and `insteadOf`. Fix: record each remote's resolved URL at stage and
  `ls-remote` that URL from a config-free env.
- **ADV11-8: a double-forked `setsid` daemon with closed fds still escapes (ADV10-12
  open).** Ran through `OSShell.Run`: `exit 0`, and its mark was written after the line.
  Fix: treat any new process that outlives the line as an orphan (a snapshot before and
  after), or give the line a uid or sandbox.
- **ADV11-9: the nested guard is keyed on the child's env (fork storm).** The pid file
  (`pidfile.go:68`, `main.go:1157`) lives under `StateDir(getenv)`. Ran: with a live holder,
  `XDG_STATE_HOME=S2`, a relative root, `env -i`, and a detached `( relay & )` were each
  not refused. The hook allowed `go run ./cmd/./relay`, `cd cmd && go run ./relay` and
  `go build -o r ./cmd/relay && ./r run`, and `relay resume` has the same gaps. Fix: a pid
  file at a fixed absolute path not taken from the env (`getpwuid`), refusal when the state
  root is relative, and holders matched by session.
- **ADV11-10: an acceptance line can still run the suite or spawn claude (ADV10-4
  partial).** `packet.go:178-253` refuses the listed spellings. It passes
  `scripts/bounded -- go test ./...`, `-run '^'` or `'.+'` over many packages, `make lint`,
  `make readme` (spawns claude), `go test -tags e2e`, `relay gate`/`resume`, `python3 -c`
  and `git -c alias.x='!make check' x`. Fix: an allowlist, meaning one package with a
  pattern that does not match every test, plus plain greps.
- **ADV11-11: the guards see only Bash.** The `PreToolUse` matcher is `Bash`
  (`plugin/hooks/hooks.json`), so Write and Edit reach `.git/refs`, `.git/hooks`,
  `.git/config` and the ledger. Bash is also allowed `symbolic-ref`, `branch -m`, `replace`,
  `core.fsmonitor` and `--exec-path`. These doors feed ADV11-1, 3 and 5. Fix: treat the
  guards as advisory, and make the gate independent of the shared `.git` and the ledger.

## Medium

- **ADV11-12: the account stop fires on ordinary API errors.** In `stream/account.go:17-19`,
  `429` and `401` match inside request ids (`req_011CT429xYz`), `log in` matches "catalog in",
  and `limit reached` matches a context limit. Every running row fails. Fix: read digits only in
  an `API Error: NNN` prefix, match whole words, and match "usage limit reached".
- **ADV11-13: same-as-main verify still passes the gate** (`relay/gate.go:88-97`); the merge
  needs exit 0 (ADV10-16).
- **ADV11-14: the linked commit hooks can be removed.** `linkHooks` (`git.go:361`) links them
  into the child's writable dir, and the gate never re-runs them.
- **ADV11-15: hygiene passes a tracked file over 4 MiB** by path only, with exit 0. Ran: the
  first line held a home path and an email. Binaries skip lan, domain, private, person and
  sibling (ADV10-17). Fix: stream-scan the file, or exit 2 unless it is allowlisted.
- **ADV11-16: `ls-remote` at stage has no timeout** (`effects/git.go:38-61`). Ran with a
  stalled `GIT_SSH_COMMAND`: 40 s. Fix: a context timeout, a process group,
  `BatchMode=yes`.
- **ADV11-17: wrong reds and unbounded reads.** A git stderr warning, which child config can
  cause, turns the gate unrun. Merge lines carry no root, so another repo's merge reads as
  main-moved. A measured line's output is buffered unbounded (`shell.go:80`).

## Low

- ADV11-18: allowed: `reset HEAD~1`, `reset --hard HEAD~1`, `commit --no-ver`,
  `-c commit.noverify` (ADV10-20).
- ADV11-19: the real-tree scanner (`cmd/relay/ar17d_test.go:15`) misses
  `filepath.Join("..","..",...)`. Five tests read the real tree (ADV10-22).
- ADV11-20: the hook answers `{}` to non-JSON input, and a crashing binary fails open.
- ADV11-21: `d9fa35c^:design/packets/rows-2026-09-23.json` still holds a home path in
  published history (ADV10-8's residue).

## TLA gaps

The pid file is not modelled (`NoNestedSpawn` covers only the env guard). `MoveMain` assumes
only the relay writes `merged`, so ADV11-1 is outside TLC. Killing parallel rows on an
account stop is Go-only. Two relays from two shells on one root are allowed by the pid file,
and no test checks `OneMeasurePerRoot` for that case.

## Verdict

**No, the relay is not ready to carry example-app.** AR17 closed the spellings ADV10 named. But
ADV11-1, 2 and 5 each put unverified content on main. ADV11-3, 5 and 6 run the child's
code with the owner's powers during the gate or the merge. ADV11-4 neuters the verify, and
ADV11-9 reopens the fork-storm door by env. example-app has commit hooks and a remote, so 5 and 6
bite first. Fix before carrying it: 1 to 7, 9, 10 and 12. The fix that matters most is one
rule: the gate and the merge read nothing the child can write. That means a hermetic git (no
hooks, no repo config), merge shas held in memory, main recorded for every row, and a commit
that must equal the verified tree. 8 and 11 can wait behind that rule, as can the other
mediums and the lows.

## The three tests to add first

1. `TestADV11_GateHermetic`: a child writes `post-checkout`, `pre-commit` and a `GNUmakefile`.
   The gate is red or unrun, and main's tree is the verified tree.
2. `TestADV11_MainBoundEveryRow`: a forged `merged` line, and a base other than main. Each
   case is main-moved and nothing merges.
3. `TestADV11_NestedByEnv`: under a live relay, `XDG_STATE_HOME` changed, `env -i`, and a
   `go build` binary are each refused.
