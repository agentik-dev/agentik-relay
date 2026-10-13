# Review adv17: agent-relay, the code since adv-16 (eb9856c..main, the ar26/ar27a/b/c chain)

Probes: guard verdicts (`hooks.VerdictEnv` and `PreToolUse` in a temp copy of the module, temp
HOME and XDG dirs), a read of Go's `modfetch/sumdb.go` in the pinned GOROOT, and a fake
`golang.org/toolchain` module served from a temp `file://` proxy. No live helper; nothing planted
in the real cache, a site dir or the real home.

## The two questions (the owner's standing word, 2026-09-24)

1. **Is the relay ready to carry example-app at `--parallel 1`, sandbox open? Qualified yes**, once
   ADV17-1 lands (a one-word `,off` on `GOPROXY`). The fork-storm doors ADV16 named are closed and
   hold (below). The remaining blockers are not relay-guard gaps: ADV17-1 (a residual of the
   ADV16-5 fix, trivial), and example-app's own pins (the TLA jar and the JVM outside the pin, AR27A's
   fork, the owner's to fix in example-app). example-app has still not been run green under the pin; that run
   is the orchestrator's after this row, and its baseline is red-on-main by ruling.
2. **Did the ADV16 chain regress anything that held at adv-16? One: ADV17-1.** The change from
   `GOPROXY=off` to `GOPROXY=file://<cache>` (ar27a act 2) opened an unchecked-toolchain path that
   `off` kept shut. Every other ADV16 fix holds, and nothing the earlier rounds closed reopened.

## The ADV16 fixes as shipped (all in a1c534c)

- **ADV16-1 (helper is a row; a marker is only read): holds.** At d=1 `read`, `for`, `print -v`,
  `(( ))`, `unset`, `local/declare/typeset NAME=`, `: ${NAME:=x}` and `IFS=; read` naming a marker
  are each denied (`markers`). `CLAUDE_SESSION_HELPER` is a row (`inRow`), so a helper's own line is
  guarded; `HELPER=yes` (not `1`) is denied. The `${n}_ROW` expansion still passes, the documented
  AR27B residual (harmless: the helper line sets its two prefix words literally).
- **ADV16-2 (helper line an allowlist; `.claude/` shut): holds.** `command`, `env`, `time`, `nice`,
  `caffeinate`, `--settings`, a `CLAUDE_CONFIG_DIR=` prefix and a second prompt word are each
  denied; `.claude/…` writes inside the project dir are denied in a row (file door and Bash), and
  the gate reds a branch touching `.claude/settings*.json`.
- **ADV16-3 (a claude by its real file): holds** in both the guard (`claudeKey`, EvalSymlinks) and
  the breaker (`Watchdog.helpers` reads `Exe` when comm is not claude). `exec -a`, `command claude`,
  the version file's path, a symlink and a bare version name are denied; the breaker's `lsof` path
  is pinnable here (`/usr/sbin/lsof`, root:wheel).
- **ADV16-4 (background flag): holds** for the tool's real value. `run_in_background: true`/`"True"`
  with a claude word is denied (ADV17-3 is the truthiness edge).
- **ADV16-5 (example-app under the pin): the relay causes are addressed** (staticcheck bound in `bin/`
  per scratch, GOBIN in the scratch, modules through `go.sum`), **but ADV17-1 is a new hole in the
  fix** and example-app's own pins are unbuilt.
- **ADV16-6 (sandbox socket text): holds** as a doc change (relay-r1 §15 residual (4): deny the
  relay's tmux socket to a row, add `launchctl load` and `crontab`).
- **ADV16-7 (verify's own scratch): holds.** `gate.Run` makes two `Scratched` shells, one for the
  acceptance lines and one for the verify line, so no acceptance write reaches the verify's cache.
- **ADV16-8 (wrong denies): holds.** `command -v claude`, `claude --version`, `type`, `which` pass.

## High

- **ADV17-1: `GOPROXY=file://<cache>` with `GOSUMDB=off` serves a toolchain unchecked; `off` did
  not** (`effects/pin.go:229-231`; regression of ar27a act 2, ADV16-5). Go's `modfetch/sumdb.go`
  Exception #1: a single `file://` GOPROXY with no `,`/`|` sets `must=false`, so a module the
  branch's `go.sum` cannot cover — and `golang.org/toolchain@…` is never in `go.sum` — is taken from
  the proxy with no checksum database and no `go.sum` line. The proxy is the account's download
  cache, which is account-writable (`~/go/pkg/mod/cache/download`, owned by the account, owner-write)
  and lies outside every pinned root and every guard. Ran, in a temp world: with the pinned shape
  (`file://<dir>` alone, `GOSUMDB=off`) and `GOTOOLCHAIN=go1.99.0`, a planted fake `go` in a
  `golang.org/toolchain` zip was extracted and executed (`PLANTED go version`); the same with
  `GOPROXY=<dir>,off` is refused (`verifying module: checksum database disabled by GOSUMDB=off`).
  Under the old `GOPROXY=off` a toolchain switch failed hard (`toolchain not available`), so `off`
  closed this; `file://` opened it. The pinned `GOTOOLCHAIN=local` narrows it — a line that does not
  re-export `GOTOOLCHAIN` uses the local go and never switches, so agent-relay's own `make check`
  is unaffected — but a recipe exporting `GOTOOLCHAIN=go1.27.0` (relay-r1 §6 records example-app does
  this) overrides the pinned value for that command and re-enables the switch. So the exact line the
  relay is readied for (example-app) is the one that opens the door. relay-r1 §6's toolchain probe
  ("exits 1, toolchain not available") holds only because the cache now holds no toolchain zip, not
  because a planted one would be caught. **Fix:** append
  `,off` to `GOPROXY` (`file://<cache>,off`). It leaves every in-`go.sum` module served (those are
  verified against `go.sum` locally, no sumdb consulted) and makes any module outside `go.sum`,
  toolchains included, fail closed. Verified both halves above.

## Medium

- **ADV17-2: act 0's copy-in is advisory, so the committed packet's provenance is unverified**
  (`prompt/rules.txt`, `template.go` CONTEXT `your packet:`; `night.go:527` reads the packet from
  `p.Root`). Nothing enforces the copy-in: a child can skip act 0 or write a *different* valid
  packet there, with no gate check that it equals the source the row names. This is no escape — the
  prompt the child runs and the acceptance/may-touch the gate reads both come from the queue and
  packet loaded once from the root at night start, which a worktree child cannot rewrite, and a
  packet that fails to parse reds `TestAR20_5_SpentStems` at the row's own gate. What is lost is
  audit: `design/packets/<row>.md` cannot be trusted to be the packet the orchestrator issued.
  **Fix (a later row, not a stop):** if a packets source is configured, have the gate compare the
  committed copy against it (the AR26 fork sketches this).

## Low

- **ADV17-3: `run_in_background` truthiness is narrow** (`guards.go`, the background read: `true`
  or `"true"`). Ran: `true` and `"True"` are denied; `1`, `"1"` and `"yes"` are not. The Claude
  Code Bash tool sends a JSON boolean, so this is theoretical, but the read should treat any
  non-false, non-empty scalar as true to fail closed.
- **ADV17-4: the breaker degrades silently without a pinned `lsof`.** `Watchdog.helpers` reads
  `Exe` only when comm is not `claude`; with no `/proc` (macOS) and no pinned `lsof`, `Exe` errors
  and a renamed claude is counted by comm alone (as before ADV16-3). Here `lsof` is pinnable, so it
  holds; on a host whose `lsof` is not in a root-owned dir the ADV16-3 breaker half is off with no
  signal. **Fix:** at `doctor`, warn when `Pinned.Lsof` is empty and `/proc` is absent.

## The fork-storm question

The doors ADV16 named are shut and stay shut: a marker cannot be dropped or reassigned, a helper is
a row and its line is an allowlist, a claude is keyed by its real file in guard and breaker, and the
background flag is read. Unbounded nesting through those is closed. What remains is the sandbox's,
already named (a double-forked `setsid` daemon, a backgrounded *script* that runs claude inside
itself — the guards read the line, not the script) and the account-writable download cache (AR27A's
open residual, of which ADV17-1 is one exploitable instance until `,off` lands).

## Verdict

**Qualified yes at `--parallel 1`, sandbox open, once ADV17-1 lands; no ADV16 fix regressed but
ADV17-1 is one.** The ADV16 chain is whole and holds under probe. The single security regression is
ADV17-1, a one-word fix (`,off`) that closes the unchecked-toolchain path `off` used to close and
that the file proxy reopened for exactly the example-app case. example-app's own pins (the JVM and the TLA
jar) stay the owner's to close in example-app and are not relay-guard gaps. Nothing here calls for a new
hardening *round*: ADV17-1 is a targeted fix, ADV17-2 an audit hardening for a later row, ADV17-3
and -4 are low.

## The three tests to add first

1. `TestAR28_ToolchainFailsClosed` (effects): with the pinned env plus a re-exported
   `GOTOOLCHAIN=go1.99.0`, a fake `golang.org/toolchain` zip in the file-proxy dir is refused
   (`checksum database disabled by GOSUMDB=off`) once `GOPROXY` carries `,off`; without `,off` it
   would run. Guards the ADV17-1 fix against a revert.
2. `TestAR28_BackgroundTruthy` (hooks): `run_in_background` as `1`, `"1"` and `"yes"` with a claude
   word is denied, alongside `true`/`"true"`.
3. `TestAR28_PacketMatchesSource` (gate): when a packets source is configured, a gate over a branch
   whose `design/packets/<id>.md` differs from the source is red; equal is green.
