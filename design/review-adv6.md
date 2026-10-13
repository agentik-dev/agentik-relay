# ADV6: the adversarial review of the relay at d727cc2

Two read-only `claude-opus-5-5` reviewers, one slice each (effects, night, stream; cmd, gate,
packet, hooks, scripts). Each reproduction ran in a `mktemp -d` copy or a temp repo, never
the real queue. "Re-run" marks the ones the coordinator repeated. Nothing is fixed here.

## High

### ADV6-1: the depth guard stops measuring, not spawning
`internal/night/night.go:230`, `cmd/relay/main.go` run verb: no `effects.Nested` check
(the only callers are plan.go:199, stage.go:181, gate.go:66, :422; re-run by `git grep`).
A child has `RELAY_MEASURE_DEPTH=1` and may run `relay run --rows x` (the packet rule admits
it), which spawns a grandchild `claude -p` whose gate reads nested and red; nothing bounds the
depth. Repro: `night.Run` with getenv depth=1 gave `spawns=1 rows=r1=red`. Fix: `relay run`
and `night.Run` refuse (exit 2, `nested`) under the depth var or `RELAY_ROW`; a hook guard
denies `relay run` in a sitting.

### ADV6-2: a killed relay leaves every child and measurement unsupervised
`internal/effects/spawn.go:79`, `shell.go:53`: `Setpgid` on every child, no `signal.Notify`
anywhere (re-run: `git grep` exit 1). SIGINT or SIGTERM to the relay never reaches the
groups, and wall, watchdog, sweep and gate all die with it: the D5 orphan shape. Repro: a
self-exec stand-in relay got SIGINT; its spawned and measured `sleep 40` both `alive=true`.
Fix: trap INT/TERM/HUP and TERM-then-KILL every live pgid; record the pgids in the state dir
so the next start reaps them.

### ADV6-3: the gate judges the worktree's HEAD but keeps or merges the branch by name
`internal/gate/gate.go:74-101`, `:454`. Acceptance lines and `clean` run on whatever the
tree has checked out; touch and merge read `st.Branch`. A child that commits on its row
branch, then `git checkout -b evil` with the content the lines want, gates green while the
kept branch fails its own line. Repro: scratch `TestZZOtherBranch`: acceptance green,
`outside=[]`, `decide=green`. Fix: require `symbolic-ref HEAD` to be the row branch and
`HEAD == <branch>`; record that sha and merge the sha (this also closes the gate-to-merge
TOCTOU).

### ADV6-4: a rename hides the deleted path from may-touch
`internal/effects/git.go:169`, `diff --name-only base...branch` with rename detection on.
Re-run: `git mv secret/keep.txt allowed/keep.txt` lists only `allowed/keep.txt`;
`--no-renames` lists both. `relay gate` on a fixture read `"outside":[]`, green. Moving
`Makefile` under an allowed path works the same way (feeds ADV6-10). Fix: `--no-renames`, or
`--name-status` judging source and destination.

### ADV6-5: same-as-main compares exit and last line only
`internal/gate/gate.go:212`, `:466`, `:172`. `make check` stops at its first failing target,
so with main red at `test`, a branch that also breaks `vet`, `doclint` or `hygiene` reads the
same exit and last line: `same-as-main`, green. Repro: scratch `TestZZSameAsMainMasks`:
`last="FAIL pkg/a"` both sides, `decide=green`. Fix: a red baseline never passes a
merge-on-green; if kept, compare per-target results (`make -k`), not the last line.

### ADV6-6: the PreToolUse guards are bypassable; `--no-verify` skips pre-push
`internal/hooks/guards.go:26-27`, wrappers at `:99`. Re-run: `relay hook PreToolUse` allowed
`timeout 60 git push --no-verify origin HEAD`, `git -c alias.p=push p origin`,
`git commit --amen -m x` (git takes the prefix); plain `git push` was denied. The reviewer
also passed `xargs`, `find -exec`, `nice -n`, `stdbuf`, `python3 -c os.system`, and landed
a `--no-verify` push in a temp repo. Fix: deny `--no-verify` and `-c alias.*`, add those
wrappers, match `--amend` by prefix; structurally, give the child an invalid `pushurl`.

## Medium

### ADV6-7: a `setsid` holder of the child's stdout hangs `RunRow` forever
`internal/night/run.go:199`: `stream.Parse(out)` blocks; the wall and watchdog kills signal
the group only and never close `out`. Deeper than ar10 handover 2 (the watchdog does not
save it). Repro: a `setsid` python holding stdout 12 s with `Wall: 2s` returned at 12.0 s;
with `sleep(inf)` never. Fix: after kill plus `KillGrace`, close the read end; `stop: orphans`.

### ADV6-8: a `setsid` descendant escapes OSShell's sweep and timeout
`internal/effects/shell.go:54`, `:68`. Repro: a line that forks a `setsid` sleeper and echoes
`ok` returned `exit=0` with the sleeper alive; memoised green. Fix: walk the ppid tree of the
line's descendants, or treat a pipe still held after `PipeGrace` as `orphans`, not clean.

### ADV6-9: the memo is keyed to a stale HEAD, and loses writes
`internal/night/bound.go:220`, `:291`: HEAD is read once at `newBound`; a commit mid-line is
stored under the old head (repro: `memo at old head ... last="two"`, tree at that head reads
`one`). `LockRoot` (`:156`) is an in-process `sync.Map`; `Save` (`:98`) writes one fixed
`.tmp` name. Repro: two copies, last writer wins; 4 writers x 200 rounds gave 598 save
errors and 7 corrupt files (read back as empty: remeasure, not a wrong green). Fix: re-read
HEAD and status after each line and store only if unchanged; flock under the state dir,
held across the merge; `os.CreateTemp` plus merge-on-save.

### ADV6-10: the gate trusts the child's packet and Makefile
`cmd/relay/main.go:522-530` (and `loadPlan` `:418`, `:431`) load queue, config and packet
from the cwd's toplevel; `plugin/commands/gate.md` runs in the session cwd. Repro: from the
row's worktree, a child that widened its own may-touch gated green; from the root, red.
`gate.go:196` resolves `verify: auto` in the child's tree and a failed probe is `skipped`
(passes): moving `Makefile` away gated green with main red. Fix: load from `fx.Git.Root`
(main); resolve the verify line once on main; a failed probe is `unrun`.

### ADV6-11: `ChildEnv` strips `RELAY_DEVICE_VERBS`
`internal/effects/spawn.go:47` drops every `RELAY_*`; `hooks/guards.go:43` reads the var in
the child's hook. Repro: `ChildEnv([... RELAY_DEVICE_VERBS=example-app,nas-admin ...])` dropped
it. The owner's device verbs never reach the child: the one guard example-app most needs. Fix:
keep it in the allow-list, or have the hook read device verbs from config.

### ADV6-12: the unbounded-acceptance-line rule is shallow
`internal/packet/packet.go:141-147`. Repro: `sh -c 'make check'`, `make -k check`,
`bash -c "go test ./..."`, `go test -run . ./internal/...`, `sh -c 'relay run'` all
accepted; `legacyRE` grandfathers any future `ar2-*`, `a1-*`, `ux9-*` id (re-run: `True`).
The depth var is the real guard; this is depth. Fix: reuse `hooks.segments`/`normalise`,
refuse any `...` pattern and `make ... check`, list legacy stems explicitly.

### ADV6-13: `--parallel` is uncapped and the breaker reads a lagging load
`cmd/relay/main.go:760` rejects only `< 0`; `effects/load.go` reads `vm.loadavg` 1-minute,
once per spawn, so a burst all passes. By reading. Fix: cap at `hw.ncpu`; gate also on a
live process count.

## Low

- ADV6-14: hygiene misses symlink targets, a file with a leading NUL, and has no email group
  (`scripts/hygiene:109-115`; repro exit 0 on all three). Fix: readlink, text ratio, email.
- ADV6-15: an absolute home-directory path in seven tracked `design/packets/rows-*.json` (re-run:
  `git grep -c`), blessed by `.hygiene-allow:13` (`home,person`). Fix: repo-relative paths.
- ADV6-16: packet ceiling off by one: `packet.go:98` refuses `>`, doclint `>=` (re-run); a
  9,216-byte packet plans and then reds main's doclint. Fix: `>=`.
- ADV6-17: `stream.Parse` reads unbounded lines (`stream/stream.go:202`). Fix: cap, `NonJSON`.
- ADV6-18: the guard is an environment variable only; `cmd/relay/main_test.go:946` builds a
  scratch env and runs a non-nested `relay plan` under the gate (safe today: temp repo). Fix:
  an ancestor pid file under the state dir as a second key.
- ADV6-19: `plugin/hooks` run `${CLAUDE_PLUGIN_ROOT}/hooks/run` unquoted (make check's
  validate warns 3x); a path with a space splits. Fix: quote it.

Sound: yaml refuses duplicates, tabs, anchors, tags; row ids are shape-checked; cmd tests use
fixture toplevels; pgid and `KillGroup` (refuses <= 1) are right; numeric flags are > 0.

## Verdict

Not ready to carry example-app. The recursion class is closed for measurement but open for
spawning (ADV6-1) and for a dead relay (ADV6-2): the fork storm can come back in a new shape.
The gate has four wrong greens a builder can reach alone (ADV6-3, -4, -5, -10), and the
child's device guard is stripped (ADV6-11), which matters most once the row is example-app. Fix
before example-app: 1 to 6, 10, 11. ADV6-7 to 9 and 12 to 13 belong in the same packet but can
follow; the lows wait.

## The three tests to add first

1. `TestADV6_1_NestedRunRefuses`: `night.Run` and `relay run` under `RELAY_MEASURE_DEPTH=1`
   exit 2 `nested` with zero spawns.
2. `TestADV6_3_GateBindsBranchSha`: a worktree on another branch, a rename out of a
   forbidden path, and a widened packet in the tree: each reads red, and the merge takes the
   gated sha.
3. `TestADV6_2_SignalReapsGroups`: a self-exec relay with a spawned child and a measured line
   gets SIGTERM; both groups are gone within `KillGrace`.
