# ADV14: the sixth adversarial review of the relay, 7d2b60f..9b99873

Target: AR22 (`ar22-fixes-adv13`, the pinned reader and the port lock) and AR23
(`ar23-stop-scope`, Stop and SessionStart only in a sitting). Two read-only Opus
reviewers (the pinned reader; the lock and the hooks) and the coordinator (the
toolchains). Every repro ran in a temp world (temp HOME, XDG_CONFIG_HOME,
XDG_STATE_HOME), in a `git archive` copy, on a test port of 49152 or above, or as a hook
decision from a built binary. The one write outside a temp dir was a touch-and-remove of
a marker in the account's pyenv site-packages, to prove it writable.

## The ADV13 fixes as shipped

Holds: 1 (`GIT_NO_REPLACE_OBJECTS`, a fresh clone), 4 (unlink, rename or a moved dir
gives no second holder; no second 127.0.0.1 bind under any `SO_REUSE*` mix), 5, 6, 7,
10, 11. Partial: 2 (git, make, sh and ps are pinned; python and go reach account files:
ADV14-1, -2), 3 (the go env file and global git config are closed; pyenv's version files
and `go.work` are not: ADV14-1, -3), 8 (the ledger is only printed, but the store's newest
file picks the session: ADV14-8), 12 (a descendant cannot free the lock; an orphan of
resume's exec keeps it: `sh -c 'sleep 60 & exit'` held the port and the next relay was
refused, "names no live relay"). Open, deferred by AR22: 9, 13 (a new route: ADV14-9).
No regression.

## High

- **ADV14-1: the gate's `make check` runs a python the sitting chooses.** `Makefile:2`
  resolves `PY` by `pyenv which python3`; the pinned env hands make `pyenv` and
  `PYENV_ROOT` (`pin.go:44`, `:168`), so the sha-bound `bin/python3` is never run. Ran
  (pinned-like env, a doc over its cap): `make doclint` exit 2; `~/.pyenv/version`
  naming a new `versions/evil` whose `python3` exits 0: exit 0; a committed
  `.python-version` naming it: exit 0; a `sitecustomize.py` in the pinned binary's own
  site-packages (same sha256): doclint 1 to 0. Reviewer: a `.python-version` in the
  account home works too, and the file door allows each write. `hygiene` is the public
  scan. Fix: drop `pyenv` and `PYENV_ROOT` from the pin, so the Makefile falls to the
  pinned link, and digest the interpreter's prefix (ADV14-2). A sandbox closes the
  writes, not the committed selection.
- **ADV14-2: the pin binds the go launcher, not GOROOT** (`pin.go:136-149`, `Moved`
  `:219-240`). GOROOT is the account's (the Homebrew Cellar), and the door allows it.
  Ran: a GOROOT copy with the same `go` bytes, one line in `src/testing/testing.go`
  (`Fail` returns): a failing `go test` went `FAIL` to `ok`, exit 0, under `GOENV=off`,
  `GOFLAGS=`, `GOTOOLCHAIN=local` and a fresh `GOCACHE`. relay-r1 §6 calls this a
  residual only confinement closes; at `--parallel 1` no sitting lives during a gate, so
  a tree digest of GOROOT and the interpreter's prefix, bound in `Pin` and read in
  `Moved`, closes it without the sandbox. The sandbox closes the `--parallel` race.

## Medium

- **ADV14-3: `GOWORK` is not pinned** (`pin.go:157-176`). The gate's clone lies under the
  account home; a sitting's `~/go.work` made every `go test` there `setup failed`, exit 1
  (reviewer ran; a wrong red on every gate). A `replace` redirect for a module with
  dependencies is unproven. Fix: `GOWORK=off`.
- **ADV14-4: a wildcard listener planted during a night blocks every later night**
  (`night/pidfile.go:185-204`). Ran on a test port: while the relay held 127.0.0.1:P,
  `net.Listen(":P")`, `0.0.0.0:P`, `python3 -m http.server P` and `nc -l P` each bound;
  after release the next relay was refused, naming nobody. The Bash guard allows the
  python line. Fix: also hold `0.0.0.0:P` and `[::]:P`; a refusal names the listener's
  pid. A sandbox that denies `network-bind` closes it.
- **ADV14-5: an interactive session is still blocked, through tmux**
  (`effects/tmux.go:41-42`, the relay's whole env). A relay started from a depth-1 session
  starts the tmux server with `CLAUDE_SESSION_DEPTH=1`; ran on a private socket: a later
  `new-window` and `new-session` inherit it, and `relay hook Stop` there blocks a dirty
  tree. Fix: run tmux under ChildEnv's drop. A sandbox does not close it.
- **ADV14-6: a sitting escapes Stop, and every guard, by swapping `RELAY_BIN`'s file**
  (`plugin/hooks/run:7-9`). The file is the account's `go/bin/relay`; ran: Stop `block`
  on a dirty repo, then the `mv` swap (allowed), then Stop `{}` and `git push` `{}`. The
  owner's next `relay run` runs the swapped binary. Fix: the shim takes `RELAY_BIN` only
  when `locked()`. A sandbox closes it.
- **ADV14-7: the settings that set the hooks' env are writable, and a committed one
  outlives the row** (`guards.go:396-428`; the spawn line, `plan.go:684`, pins no
  settings). Ran: Write to the worktree's and the root's `.claude/settings.local.json`
  and the home's settings: allowed; the hook under `RELAY_ROW=` and
  `CLAUDE_SESSION_DEPTH=0`: Stop `{}`. Not run: whether a live session reloads them. A
  merged `.claude/settings.json` with `disableAllHooks` reaches every later sitting.
  Fix: spawn with pinned `--settings` and `--setting-sources`, deny
  `.claude/settings*.json` in the door, red a branch that touches `.claude/`.
- **ADV14-8: resume opens the newest transcript, which a sitting can plant**
  (`cmd/relay/main.go:1375-1414`). Ran: a newer `<uuid2>.jsonl` in the row's store dir
  turned the pick to uuid2; the door allows it. Fix: resume the ledger's `session_id`
  only when the store holds that uuid, else the picker. A sandbox closes it.
- **ADV14-9: ADV13-13 is open with a new route** (`guards.go:57-113`,
  `pidfile.go:26,43-50`). Ran (hook decisions `{}`): `cp -R cmd/relay internal/r2` and
  `go build -ldflags "-X …LockPortLink=4242"`, then `env -u RELAY_ROW /tmp/r run`; `go -C
  cmd/relay build`, `cd cmd/relay && go build` and `go test -c ./cmd/relay` pass too. A
  probe main built so printed port 4242, not 20501: its own lock. The e2e harness uses
  that `-X`, so a builder may copy it. Fix: the link vars behind a test build tag, so
  every build takes the one port; an edited source still escapes (the sandbox).

## Low

- ADV14-10: `LockMoved` reads only (dev, inode): an in-place rewrite of `relay.pid`, a
  hard link and restore, or a `mv` away and back leave it `""`, and a refusal can name a
  planted non-relay pid (ran). A directory at `relay.pid`'s path fails every later verb,
  `file exists` (ran). The relay-home sweep follows a symlink (ran; the state root's door
  blocks the realistic path). `CLAUDE_SESSION_DEPTH` of `" 1"`, `"1.0"` or twenty digits
  is not a sitting though §10 says an integer of 1 or more (only the spawner sets it).
  §8 still carries AR21's flock paragraph. `/usr/bin/git` and `make` are xcrun shims
  that read a same-uid cache (not run). The stand-in's sittings carry
  `CLAUDE_SESSION_DEPTH=1` and no `RELAY_ROW` (this one, read from its own env), so
  push, nested-run, row shapes and the file door are off in its rows: the orchestrator's.

## The hooks question

A sitting cannot look interactive by its own env (set at spawn; an `export` never
reaches the hook), but it can by the binary (ADV14-6) or the settings (ADV14-7). An
interactive session is still blocked through the tmux server (ADV14-5); a hand-run
`claude-session start` is blocked as ruled. The split in `hooks.go` matches §10's lines.

## TLA gaps

`Holders` counts only relays and under the port `Seen = Holders`, so a non-relay holder
(ADV14-4, ADV13-12's orphan) and the refusal "held, no live relay" are unreachable in the
model. Resume's exec is not modelled; no action reads `unlinked`, so the `lock-moved`
stop and its evasions (ADV14-10) are outside it; a relinked relay (ADV14-9) always
passes `PidGuard`. No invariant names the toolchains' trees (ADV14-1, -2). `make tla`
was not run (28 minutes at AR22).

## Verdict

**No: the relay is not ready to carry example-app, even with the sandbox decision open.**
ADV14-2 decides it: one Write under the account's GOROOT turns a example-app sitting's
failing `go test` green with the launcher's sum unchanged, and a tree digest closes it
at `--parallel 1` without the sandbox. ADV14-1 is the same hole for the plugin repos'
python (example-app's Makefile runs none). The pinned reader holds for git, make, sh, ps,
git config and refs, the go env file and the ledger; what leaks is the toolchains'
trees and two env keys. Fix before example-app: ADV14-1 to -3, one act (digest GOROOT and
the interpreter's prefix in `Pin` and `Moved`; drop `pyenv` and `PYENV_ROOT`;
`GOWORK=off`). Before a night runs beside the owner's sessions: -4 to -8. The
sandbox's: -9, -10's rest, and the §6 `--parallel` race.

## The three tests to add first

1. The toolchain test: `PinAt` over a temp GOROOT copy; one byte changed under
   `src/testing`; `Moved` names go and the gate is unrun `tool-moved`.
2. The pyenv test: under the pinned env, `PYENV_ROOT/version` and a committed
   `.python-version` naming a fake interpreter; the Makefile's `PY` is `bin/python3`.
3. The wildcard test: a `:P` listener while the lock is held fails to bind, or the next
   refusal names its pid; and a `go.work` above the clone changes no gate line's exit.
