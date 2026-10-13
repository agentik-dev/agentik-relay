# Review adv16: agent-relay, the code since adv-15 (0dba7dc..main, AR25)

Probes: guard verdicts (`hooks.VerdictEnv` in a temp copy of the module, temp HOME, XDG dirs
and GOCACHE), zsh with a dummy variable, and a read-only `go env` under the pin's env lines. No
live helper; nothing planted in a cache, a site dir or the real home.

## The ADV15 fixes as shipped

- ADV15-1 (high): **partial.** The markers now ride through assignment, `env`, `unset` and
  `export` forms, but three doors stay open (ADV16-1 to -3). Not a regression.
- ADV15-2 (high): **holds for its door.** `tmux`, `screen` and the rest are denied as a command
  word. The relay has its own socket, and the merge reads a
  strict `Moved`. The class is §15 (4)'s.
- ADV15-3 (high): **holds.** `PYTHONNOUSERSITE=1` is set. Each gate, baseline and line runs under
  a fresh `MkdirTemp` scratch home, so only `bin/` outlives a line, and `Moved` reads `bin/`. A
  residual inside one gate is ADV16-7.
- ADV15-4 (medium): **open.** `relay doctor --verify` exists (e9d127b); no run was made. ADV16-5.
- ADV15-5 (medium): **holds.** The relay's server is on its own socket, and a sitting's tmux is
  denied.
- ADV15-6 (medium): **open.** It is named in §15 (1) and (2) and not built.

## High

- **ADV16-1: zsh's own assignments blank a marker, and the helper they start has no row guard**
  (`guards.go` `envEdits`, `markers`; `guards()` `inRow`). `envEdits` reads `NAME=`, `env`,
  `exec -c` and the `unset`/`export` family only. Ran `VerdictEnv` at d=1: allowed are
  `read RELAY_ROW </dev/null`, `for RELAY_ROW in ''`, `print -v`, `printf -v`,
  `(( CLAUDE_SESSION_DEPTH = 0 ))`, `getopts`, `vared`, `mapfile`, `: ${RELAY_ROW::=}` and
  `zsh -c 'read RELAY_ROW …'`. Measured in zsh with a dummy `T`: `read`, `for`, `print -v`,
  `printf -v` and `(( ))` each change the value a child reads. So `read RELAY_ROW </dev/null;
  CLAUDE_SESSION_DEPTH=2 CLAUDE_SESSION_HELPER=1 claude -p hi --max-turns 1` is allowed, and that
  helper's hook sees no row. Ran with RELAY_ROW blank at d=2: `claude -p hi &`,
  `env -u GIT_CONFIG_COUNT git push origin HEAD`, `tmux new -d claude -p hi`, `relay run --rows x`
  and `claude-session start` are all allowed. That is unbounded nesting and a push. Fix: fail
  closed. A non-empty `CLAUDE_SESSION_HELPER` is a row (`inRow`), and an unreadable depth under it
  reads as `MaxHelperDepth`. In a row, deny any word that names a marker outside a `$NAME` read.
- **ADV16-2: the helper line admits every other word, including the ones that turn its hooks
  off** (`helperLine`). Ran at d=1: allowed with `--settings '{"disableAllHooks":true}'`,
  `--setting-sources project`, a `CLAUDE_CONFIG_DIR=/tmp/x` prefix, `--dangerously-skip-permissions`,
  `--agents`, `--resume`, `--max-turns 99999999` and `exec -a zz`. The row's hooks reach a sitting
  only through the user-level plugin (the spawn passes no `--settings`, `plan.go:690`). The pinned
  claude binary carries `disableAllHooks`, `setting-sources` and `CLAUDE_CONFIG_DIR`, so each of
  the first three starts a helper whose hooks never load. Even the exact line gets none once the
  sitting writes `.claude/settings.local.json`: `printf … > .claude/settings.local.json` is
  allowed, and no gate rule reads a branch's `.claude/` (ADV14-7, still open). Committed, that file
  merges, and every later row starts without hooks. A helper also needs no `--max-budget-usd`, so
  its spend is outside the row's. Not run: a live helper. Fix: make `helperLine` an allowlist
  (`-p`, `--max-turns` up to the sitting's remaining turns, `--max-budget-usd` required, `--model`,
  `--output-format`, `--verbose`), with no prefix but the two. In a row, deny a write under
  `.claude/` (file door and Bash), and have the gate red a branch that touches
  `.claude/settings*.json`.
- **ADV16-3: a claude under another name passes the guard and the breaker** (`helper`;
  `procs.go` `IsClaude`). The guard keys on the command word's base name `claude`. The breaker
  keys on `ps -o comm`, which is argv[0]. Ran at d=1: allowed are `<versions>/2.1.282 -p hi` (the
  pinned claude's real file), `exec -a zz <versions>/2.1.282 -p hi`, `ln -s <versions>/2.1.282 c;
  ./c -p hi` and `npx @anthropic-ai/claude-code -p hi`. Measured: `exec -a notsleep sleep` reads
  comm `notsleep`, and a symlink reads its own path, which `IsClaude` does not resolve. Each such
  claude is a full sitting at depth 1 that can do the same again: a chain the depth never bounds
  and the breaker never counts. §15 (6)'s premise ("each bounded by its own `--max-turns` and the
  hooks' depth") fails under ADV16-1 to -3. Fix: the guard resolves a command word through PATH
  and `EvalSymlinks` and denies any word whose real file is the pinned claude's, and denies
  `exec -a` in a row. The breaker reads the executable path (`proc_pidpath`, or
  `lsof -p <pid> -d txt -Fn`) and compares it with the real file. npx's copy stays the sandbox's.

## Medium

- **ADV16-4: "never backgrounded" never sees the Bash tool's own flag** (`guards.go:571-575` reads
  `command`, `file_path` and `notebook_path` only). A turn of N helper lines, each sent with
  `run_in_background: true`, is N allowed helpers at once. Only the breaker's 60 s tick bounds
  them. Fix: read `run_in_background` and deny a claude command word when it is true.
- **ADV16-5: example-app is unmeasured under the pin, and the scratch home moved its staticcheck
  out of reach** (ADV15-4, -6). Its Makefile (unchanged since 2026-09-19, lines 198-212)
  calls `$(go env GOBIN)` or `$(go env GOPATH)/bin`. Under the pin's env lines that is
  `<scratch>/go/bin` (ran: `go env` gives GOPATH `<tmp>/scr/go`), which is fresh at every gate. The
  account's `~/go/bin/staticcheck` is never reached, so every gate runs `go install` under
  `GOPROXY=off` (ADV15 measured it fail), or compiles from the account's module cache.
  `GOMODCACHE` is the account's (`pin.go:214`), a gate input any sitting writes, with 54 of its
  modules. It exports `GOTOOLCHAIN=go1.27.0`: the day brew's go passes 1.27.0, it switches to a
  toolchain in that same cache, a tree outside the roots. Fix: pin staticcheck as a `PinTools`
  entry and set `GOBIN=<home>/bin` in the pinned env, send the cache through §15 (2), then run
  `relay doctor --verify <example-app>` green before any of its rows.

## Low

- ADV16-6: §15 (4) says the sandbox profile "must allow the relay's tmux socket". A sitting never
  needs it, and a window there is the relay server's child without the markers, so the profile
  must deny it. (4) also omits `launchctl load` of a LaunchAgent and `crontab`, which outlive the
  night and a reboot.
- ADV16-7: a gate's acceptance lines and its verify line share one scratch `GOCACHE`
  (`gate.go:270`). A branch test that writes the cache can hand the verify line's `go test` a
  cached pass, because Go does not re-hash a cached output. Fix: give the verify line its own
  scratch.
- ADV16-8: wrong denies. `command -v claude` and `claude --version` are denied (`helper`), while
  `type claude` and `which claude` pass.

## The helpers question

Yes: a sitting can still drop its markers (ADV16-1) and nest without bound through a helper
(ADV16-1, -2) or a claude under another name (ADV16-3). Each is an allowed line, and the breaker
cannot see ADV16-3. What holds is outside the hooks: the locks, the sums and trees, the scratch
home, the gate on main.

## Verdict

**No: the relay is not ready to carry example-app at `--parallel 1`, with the sandbox decision still
open.** ADV16-5 decides it first. example-app's check has never run under the pin, its lint cannot
reach staticcheck under the scratch home, and its modules come from a cache any sitting writes.
ADV16-1 to -3 decide the rest: each is an allowed fork-storm door. The depth does not bound it,
and for ADV16-3 the breaker does not count it either. The answer becomes yes at `--parallel 1`
without the sandbox once four things land:

1. example-app is green under the pin by `relay doctor --verify` (staticcheck pinned, the cache
   through `go.sum`).
2. The helper fails closed: `CLAUDE_SESSION_HELPER` is a row, a word naming a marker is denied,
   and `helperLine` is an allowlist.
3. `.claude/` is denied in a row and red at the gate.
4. The guard and the breaker key a claude by its real file, not its name.

None of the four needs the sandbox.

## The three tests to add first

1. `TestAR26_MarkerBuiltins`: at d=1, `read`, `for`, `print -v`, `printf -v`, `(( ))`,
   `getopts`, `vared` and `mapfile` naming a marker are each denied. With
   `CLAUDE_SESSION_HELPER=1` and `RELAY_ROW` blank, `claude -p hi &` and `git push` are still
   denied.
2. `TestAR26_HelperAllowlist`: the helper line with `--settings`, `--setting-sources`, a
   `CLAUDE_CONFIG_DIR` prefix, `exec -a`, or no `--max-budget-usd` is denied. A Bash write to
   `.claude/settings.local.json` is denied. A gate over a branch adding `.claude/settings.json` is
   red.
3. `TestAR26_ClaudeByRealFile`: a fake `Procs` gives comm `zz` and `./c` with an executable path
   equal to the pinned claude's real file, and the row trips `helper-storm`. The guard denies the
   version file's path and a symlink to it.
