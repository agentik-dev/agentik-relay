# ADV13: the fifth adversarial review of the relay, 59b087a..1c3b734

Target: AR21 (`ar21-fixes-adv12`). First: what a sitting can write that the gate or the
merge reads; then the pid file's holder rule. Two read-only Opus reviewers (guards and
packet; pid file and ledger) and the coordinator (gate and merge). Every repro ran in a
temp world (temp HOME and XDG), in an archive copy with a throwaway test, or as a hook
decision from a built binary; none touched a real config or the live pid dir.

## The ADV12 fixes as shipped

Holds: 4, 5 (the original repros are denied), 6 (`Test[A-Z]` and the four-package line
are refused), 10 (alternates digested). Partial: 1 (the runner reads no global config,
but the stage and the verify line do: ADV13-3, -10), 2 (bound once per root and checked
after the verify and the commit, but a deny-list and no refs: ADV13-1, -9), 3 (the double
fork is refused, naming the live holder; `rm` or a relink is not: ADV13-4, -13), 7
(main, digest and URLs are bound afresh; base, green lines, `head_before`, `worktree`
and `session_id` are still read as facts: ADV13-5, -7, -8, -11), 8 (file tools only).
**Regression: ADV13-6** (act 5's Afresh blinds the hand gate to a push).

## High

- **ADV13-1: replace refs judge one tree and merge another.** `refs/replace/` is in the
  common dir; `runnerEnv` (`effects/git.go:46`) lacks `GIT_NO_REPLACE_OBJECTS`, the
  digest hashes no refs, and the hook allows `git replace`. Ran (gate test on
  `globalWorld`): `git replace <bad a.txt blob> <good blob>` in the child's tree; `Run`
  green, `MergeIfGreen` merged with verify `""` and `auto`; `main:a.txt` is `a` through
  the ref and `a1 a.txt` without it; a clone's `make check` exits 2. Fix:
  `GIT_NO_REPLACE_OBJECTS=1` in the runner and every verify env; refuse any
  `refs/replace/*` at stage, gate and merge; hash `shallow`.
- **ADV13-2: the gate's programs come from a PATH the sitting writes.** `exec.Command
  ("git")` (`git.go:37`, `:168`) and the verify's `/bin/sh -c` (`shell.go:79-81`) look up
  by PATH. Measured: the relay's PATH starts with five account-writable dirs
  (`<home>/.local/bin`, `<home>/go/bin`, `<home>/.pyenv/shims`, `/opt/homebrew/bin`,
  `/opt/homebrew/sbin`), ahead of `/usr/bin/git` and `/usr/bin/make`; the Write door
  allows all of them. Ran (gate test, a temp dir for `~/.local/bin`): a `make` that
  exits 0 and a `cat` that prints `a`: `Run` green, merged with `auto`, `main:a.txt =
  "a1 a.txt"`. Fix: absolute `git`, `make`, `go`, `sh` resolved at start from dirs the
  account cannot write; verify lines get a PATH of those dirs only.
- **ADV13-3: the verify line reads the owner's home.** `shell.go:81` passes
  `os.Environ()`. Ran (temp HOME; the hook allows `go env -w`):
  `go env -w GOFLAGS=-run=^$` turns a failing `go test ./...` to exit 0;
  `-exec=/usr/bin/true` too. The verify's own git reads `~/.gitconfig`. Fix: verify
  lines under `GOENV=off`, `GOFLAGS=`, `GIT_CONFIG_GLOBAL=/dev/null`,
  `GIT_NO_REPLACE_OBJECTS=1` and a relay-owned HOME, caches named explicitly.
- **ADV13-4: a removed pid file gives a second holder (fork-storm door).** The flock
  holds an inode, not a path (`night/pidfile.go:93-105`). Ran (`PidFile` on a temp dir,
  holder live): after unlink, rename of the file, or rename of the dir, the next comer
  is held. The hook allows each step in a sitting: `rm <home>/.local/state/agent-relay/
  relay.pid`, `cp <installed relay> /tmp/r`, `env -i HOME=/x PATH=/usr/bin:/bin /tmp/r
  run`. Fix: a lock a sitting cannot unlink (a loopback port or a Mach name), and the
  holder re-stats the path each poll and stops the night on a new inode.
- **ADV13-5: the hand gate takes the base from the last `stage` line**
  (`stage.go:547`, `gate.go:729`). Ran: ledger base `main` gives red
  `outside=[evil.txt]`; base `decoy`, a branch at the child's HEAD, gives green
  `outside=[]`, and the harness overlays from it. Fix: the base is the queue row's;
  a ledger base that differs refuses.
- **ADV13-6 (regression, act 5): Afresh compares ls-remote now with ls-remote now**
  (`stage.go:581`, `gate.go:131,189`). Ran: after the child's `git push <url>
  HEAD:refs/heads/evil`, in process `failed origin-moved`; the hand gate `green,
  origin_same=true`. Fix: the line's `origin_refs` stay a claim that can only red, or
  the hand gate lists `ls-remote` as skipped.
- **ADV13-7: a forged green line lands a row that never ran** (`plan.go:195,460,482`).
  Ran: `{"event":"note","row":"a4-schemas","result":"green"}` makes `a4-schemas`
  `landed` and its dependant `ready` (any event name counts). Fix: a queued row is
  satisfied only by git (`landed`) or this process's memory.
- **ADV13-8: resume execs the ledger's `worktree` and `session_id`**
  (`cmd/relay/main.go:1253-1264`). Ran: a forged line gives exec dir `/tmp/evil`, argv
  `claude --resume --settings=/tmp/evil/s.json`, run in the owner's session, outside
  the hooks, holding the lock (claude reading it as an option is inferred). Fix: a
  worktree git lists under `.claude/worktrees/<row>`, a UUID session id, `--` first.

## Medium

- **ADV13-9: the program-key refusal is a deny-list that misses keys.** `ProgramKey`
  (`git.go:694-722`) lacks `gpg.ssh.defaultKeyCommand`; ran: with `commit.gpgSign` and
  `gpg.format=ssh`, the runner's commit ran it. A new process binds the digest afresh
  (`stage.go:461-466`), so there the list is the only barrier, and `core.worktree`,
  `core.sparseCheckout`, `remote.*.url` pass it. Fix: an allowlist of local keys, or
  a relay-owned clone.
- **ADV13-10: the stage's `plainGit` reads the global config** (`git.go:103`, `:365`).
  Ran: a global `core.hooksPath` (a sitting may `git config --global`); the next row's
  `worktree add` ran its `post-checkout` as the relay, `RELAY_ROW` empty. Fix:
  `GIT_CONFIG_GLOBAL=/dev/null` there too.
- **ADV13-11: `landed` reads `head_before` from the ledger** (`plan.go:499-501`). Ran:
  an unmoved branch off a merged tip: real false, forged true. Fix: the cut from memory
  or the merge-base.
- **ADV13-12: resume's lock passes to every descendant** (`pidfile.go:161-177`). Ran: an
  orphan of the exec'd process keeps the lock (`nested: relay <dead pid> is live`), and
  a grandchild's `LOCK_UN` frees it while the sitting lives. Fix: a small parent holds
  the lock and waits; a refusal names a live pid only.
- **ADV13-13: a relinked or edited relay picks its own pid dir** (`pidfile.go:25-39`).
  The hook allows `go -C cmd/relay build -ldflags "-X …PidHomeLink=/tmp/h"`,
  `cd cmd/relay && go build` and `go test -c ./cmd/relay`; ran: the relinked binary under
  `env -i` passes a held lock. Fix: read `go -C` and a cwd in cmd/relay, and name the
  pid file advisory against a hostile child.

## Low

- ADV13-14: Bash writes under `.git` (`echo x > ../../.git/shallow`) are allowed;
  `PidFile` needs `ps` on PATH (`env -i` fails naming itself) and never reads `started`
  back; `relay gate` takes no pid file and no env guard yet runs branch code;
  `clean --merged` can remove a live night's fresh worktree; a refusal in the
  truncate window names pid 0.

## TLA gaps

`Night.tla:263-294` assumes a dead holder's flock dies with it; ADV13-12's orphan
contradicts it, and no Go test kills a holder and takes over. The model cannot express
a lock's identity (ADV13-4), inheritance and `LOCK_UN` (-12), a per-binary pid dir
(-13), or resume. No invariant names the gate's inputs, so ADV13-1 to -3 are outside
TLC. `make tla` did not finish in 3 minutes; TLC was not run.

## Verdict

**No: the relay is not ready to carry example-app.** ADV13-1, -2 and -3 each merged a tree
main's own check rejects, with a green gate, from one line a sitting may run; ADV13-4
reopens the fork-storm door ADV12-3 closed; ADV13-6 is a regression. Every round
enumerates one more thing the child writes (global config, repo config, now refs, PATH,
the go env), and the child writes as the owner, so the list does not converge. The fix
that matters most is to pin the reader: the gate and the merge run with absolute tool
paths, a relay-owned HOME, `GOENV=off`, `GIT_CONFIG_GLOBAL=/dev/null` and
`GIT_NO_REPLACE_OBJECTS=1`, in a clone fetched by one ref. Fix before example-app: ADV13-1
to -8. Can wait: -9 to -14.

## The three tests to add first

1. The replace-ref gate test: `git replace` in the child's tree; `Run` not green,
   `MergeIfGreen` never merges.
2. The pinned-env gate test: a fake `make` and `cat` ahead on PATH and
   `GOFLAGS=-run=^$` in the temp HOME's go env; never green.
3. The takeover test: holder live, the pid file unlinked, renamed and its dir renamed;
   a second `PidFile` refused each time. Beside it, the hand gate after a push by
   URL reads `origin-moved`.
