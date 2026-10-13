# Review adv15: agent-relay, the code since adv-14 (c6233ae..main, AR24)

The seventh adversarial round. One reviewer, the coordinator alone. Every probe ran in a temp
world: HOME, XDG_CONFIG_HOME and XDG_STATE_HOME were temp dirs, tmux ran on a private socket,
and the guards were probed by `Verdict` in a temp copy of the module. No live helper was run and
no cache or site dir was planted: each finding names what was run and what was not. Nothing is
fixed here.

## The ADV14 fixes as shipped

These hold: 1 (pyenv is out of the pin, so `PY` falls to `bin/python3`; the same class reopens
through the relay home, ADV15-3), 2 (GOROOT is snapshotted; it holds at `--parallel 1` for a
file changed and left changed; the race is ADV15-2), 3 (`GOWORK=off`) and 4 (the wildcard
binds on darwin; a squatter bound before a start is deferred). Partial: 5. The relay's own tmux
commands drop the sitting's markers, but a server a sitting starts still passes them on
(ADV15-5), and the clean server is now a laundering host (ADV15-2). Open and deferred to the
sandbox (§15): 6, 8 and 9. Open: 7. Its committed `.claude/settings.json` half is not the
sandbox's, and §15's gate rule ("a branch touching `.claude/` is red") is not built. None
regressed: a sitting could drop its markers with `env -u` before AR24 as it can after.

## High

- **ADV15-1: a helper drops every row guard, and the only nesting counter with them**
  (`hooks.go:78`, `guards.go:196`, relay-r1 §10's helper rule). The hooks judge a sitting by the
  hook's own env: `RELAY_ROW`, or `CLAUDE_SESSION_DEPTH >= 1`. §10 now tells a sitting's
  `claude -p` helper to drop both. Ran, `Verdict(cmd, inRow=true)`: `claude -p hi`,
  `env -u RELAY_ROW -u CLAUDE_SESSION_DEPTH claude -p hi` and `RELAY_ROW= CLAUDE_SESSION_DEPTH=0
  claude -p hi` are all allowed. `env -u GIT_CONFIG_COUNT git status` is denied (other-tree), so
  the guards refuse dropping git's env but not the sitting's own. The helper's hooks then add no
  Push, NestedRun, RowShapes or file door, and Stop is off. It can drop the pushurl deny too,
  since other-tree is row-only. The depth is reset, so helpers nest with no bound and no
  `--max-turns` the relay sets: a fork-storm door. Not run: a live helper. Fix: split the
  markers. `RELAY_ROW` rides every descendant, and a row denies any segment that drops or blanks
  `RELAY_ROW`, `CLAUDE_SESSION_DEPTH` or `CLAUDE_SESSION_MAX_TURNS` (`env -u`, `NAME=`, `unset`,
  `env -i`), the gitElsewhere shape. A sitting's `claude` needs `--max-turns` and runs at depth
  + 1, refused past a small bound. `DropSitting` stays for the relay's own helpers alone.
- **ADV15-2: tmux gives any sitting an escaped writer that the sweep never sees**
  (`tmux.go:65,69`; `procs.go:309-317`; `tree.go` `readEntry`). The relay's server is on the
  default socket, and since AR24 it starts clean. Ran: `Verdict` allows `tmux new-window -d
  claude` in a row. That window is the server's child, never the sitting's, so it has no
  markers. The watchdog never sees it, `SweepEscaped` (descendants the watchdog saw) never
  kills it, and it outlives the row. So `--parallel 1` does not mean no writer is alive at a
  gate. The residual is the design's own (ADV11-8, the sum-to-exec race), but the door is an
  allowed command on a tool this Mac now has. The fast path also lets a write that is restored
  before the merge's `Moved` pass: a changed stat re-hashes, and equal bytes read unmoved.
  Not run: a timed write. Fix: deny `tmux` as a command word in a row; run the relay's tmux on
  its own socket (`-S` under the state root); make the merge's `Moved` strict (any ctime
  changed since the gate's `Moved` is moved). For the sandbox decision: an inherited profile
  does not reach a process that a same-uid server starts, so the profile must deny the tmux
  socket.
- **ADV15-3: the relay home is a gate input that a sitting can write** (`pin.go:170-176`,
  `guards.go:387-426`). The file door covers only the file tools. Ran: `Verdict` allows
  `touch <state>/agent-relay/home/1/cache/go-build/x`. Two dirs there feed every later gate
  line of the night, and neither is bound. (a) `GOCACHE`, kept across gates: Go does not
  re-hash a cached output on read (that is what `GODEBUG=gocacheverify` exists for). (b)
  python's user site. Ran, with the pinned python under `HOME=<relay home>`: `ENABLE_USER_SITE
  True`, site `<home>/Library/Python/3.14/lib/python/site-packages`, and `pin.go` sets no
  `PYTHONNOUSERSITE`. That dir does not exist at the pin, so it is never a root. Once made, a
  `.pth` or `usercustomize.py` there runs at every gate python start (ADV14-1's class). Not
  run: a file in either. Fix: `PYTHONNOUSERSITE=1` in the pinned env; `GOCACHE` fresh per gate,
  or snapshotted at each gate's end and read in `Moved`. Both close it at `--parallel 1` with no
  sandbox.

## Medium

- **ADV15-4: example-app's `make check` was never measured under the pin, and it reads red there**
  (example-app's Makefile: `CHECK_GATES` 337, lint 198-212, verify-spec 422-440, provider 543).
  Ran at example-app under a pinned-like env and a temp HOME: `go env GOPATH` is `<home>/go`, so
  lint looks for staticcheck in the relay home. `go install
  honnef.co/go/tools/cmd/staticcheck@2026.2.1` fails with `module lookup disabled by
  GOPROXY=off`, so lint exits 1 on main and on the branch. A red main never merges
  (`gate.go:863`), so every example-app row goes branch-only. Beyond it: verify-spec runs a JVM from
  `/opt/homebrew/opt/openjdk/bin` (account-owned, not in `PinTools`) and a jar from its search
  paths, and provider compiles 54 required modules from the account's `GOMODCACHE`. So
  example-app's gate reads three toolchains that the pin binds none of. relay-r1 §6's "measured by
  hand" covers this repo alone. Fix: measure example-app's check under the exact pinned env before
  its first row; pin java (and its `java.home` tree) and a staticcheck built at start. For the
  module cache, fill a relay-home `GOMODCACHE` through `GOPROXY=file://<account cache>/cache/
  download` with `GOSUMDB=off`, so each zip is checked against the tree's `go.sum`, which is in
  the diff.
- **ADV15-5: a server started with the markers still hands them to the owner's sessions**
  (ADV14-5's other half). Ran on a private socket: a server started under `RELAY_ROW=r1
  CLAUDE_SESSION_DEPTH=1`, then `new-window` and `new-session` from a clean env. Both carried
  both keys. A sitting may start that server (tmux is allowed), and the relay uses the default
  socket, so the owner's later interactive tmux session is a sitting to Stop and Push. Fix:
  ADV15-2's deny and the relay's own socket.
- **ADV15-6: §6 gives the sandbox two residuals that `--parallel 1` closes as it closed GOROOT.**
  Ran: `otool -L` of the pinned python's `_hashlib` names
  `/opt/homebrew/opt/openssl@3/lib/libcrypto.3.dylib`, which is account-owned and outside every
  root. The module cache is ADV15-4's. Fix: add each pinned binary's and extension's load
  closure (to the system paths) to the roots; the module cache goes through `go.sum` as
  above.

## Low

- ADV15-7: §10's helper sentence prescribes the drop that ADV15-1 turns into an escape. TLA
  gap: `Night.tla` has no process outside the relay's tree (a tmux window, a helper at depth
  0), so no Storm config can find ADV15-1 or -2.

## The helpers question

Yes: a sitting escapes its own guards by the helper rule (ADV15-1) or by a tmux window
(ADV15-2), each allowed today. What still binds it is what the relay does outside the hooks: the
port lock (no second night), the sums, the trees and the gate on main. Those hold only while no
writer outlives its sitting, and ADV15-2 is such a writer.

## Verdict

**No: the relay is not ready to carry example-app, even at `--parallel 1`, with the sandbox
decision open.** ADV15-4 decides it first: under the pin, example-app's own check reads red on main,
so no example-app row can merge, and the tools it runs sit outside the pin. ADV15-3 and ADV15-2
decide the rest: a sitting can still change what a later gate runs (the relay home's Go cache
and user site) and can leave a writer alive at the gate. The answer becomes yes at
`--parallel 1` without the sandbox once four acts land: ADV15-4 (example-app's check measured and
green under the pin), ADV15-3 (two env lines and a bound cache), ADV15-2 (tmux denied in a row,
its own socket, a strict merge `Moved`), and ADV15-1 (the markers kept). None needs the sandbox.
Above `--parallel 1` the race stays the sandbox's.

## The three tests to add first

1. `TestADV15_MarkersKept`: in a row, `env -u RELAY_ROW`, `CLAUDE_SESSION_DEPTH=0`,
   `unset CLAUDE_SESSION_MAX_TURNS`, `env -i` before `claude`, and any `tmux` command word are
   each denied.
2. `TestADV15_RelayHomeBound`: after the pin, the pinned env carries `PYTHONNOUSERSITE=1`, and
   a file changed under `GOCACHE` between two gates is `tool-moved` (or each gate's `GOCACHE`
   is a fresh dir).
3. `TestADV15_RestoredBeforeMerge`: a fake GOROOT file written and restored (same bytes, new
   ctime) after the gate's `Moved` reads moved at the merge's.
