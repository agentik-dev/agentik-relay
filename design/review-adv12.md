# ADV12: the fourth adversarial review of the relay, 74cfb98..4f189e5

Target: AR20 (packet `ar20-fixes-adv11`): the hermetic git runner, the state digest,
main bound for every row, remotes bound by URL, the pid file, the acceptance allowlist,
the push guard under `RELAY_ROW`, the Write door and the account stop. Two read-only
Opus reviewers (gate and merge; guards, packet and stream). Every repro ran in an
archive copy with throwaway tests, or through the built binary's `hook PreToolUse`,
with temp `HOME` and `XDG_STATE_HOME`. The coordinator re-ran ADV12-1 and -2 and read
ADV12-3's holder rule.

## The ADV11 fixes as shipped

Holds: 2 (main is re-read by refname for every row), 4 (every make spelling is guarded
and overlaid), 5 (`--no-verify`, and the committed tree must equal the judged tree; but
ADV12-1 poisons the tree it judges), 12, 14 (the common `hooks/` is digested), 16
(timeout, process group, `BatchMode`), 17 (see ADV12-10). Partial: 1 (ADV12-7), 3 and 6
(ADV12-1), 9 (ADV12-3), 10 (ADV12-4, -6), 11 (ADV12-1, -8). Open: 7 (ADV12-1), plus
8, 13, 15 and 18 to 20, which were deferred on the AR20 charter; 18 to 20 still reproduce.
No fix regressed. **The push word holds:** `RELAY_ROW` is read from the hook's own env,
so `env -u RELAY_ROW git push`, `RELAY_ROW= git push` and `-c remote.origin.pushurl=`
are denied in a sitting, and a push with no `RELAY_ROW` is allowed, as ruled.

## High

- **ADV12-1: the global git config is outside the runner and the digest.** `gitCmd`
  (`effects/git.go:31-34`) and `LsRemote` (`:104-116`) set `GIT_CONFIG_NOSYSTEM` but keep
  `HOME` and never `GIT_CONFIG_GLOBAL`. `GitStateDigest` (`:531`) hashes only the repo's
  config, hooks and info. A sitting can write `~/.gitconfig` with the Write door (allowed:
  `$HOME/.gitconfig`, `~/.config/git/config`) or with `git config --global` (allowed).
  Ran, two ways (reviewer and coordinator): a global `filter.fix.smudge` plus a branch
  `.gitattributes` repaired the broken `a.txt` at the fresh checkout. The result was
  `Run = green` and a merge, with `main:a.txt = "a1 a.txt"`, which main's own check
  rejects. The smudge is any program, run with the relay's env. Ran: a global
  `url.<mirror>.insteadOf <origin>` after a push by URL gave `origin_same=true`, so every
  remote compare, before and after the lines and after `mergedVerify`, reads the mirror.
  The gate tests hide all this: `gate_test.go:56` pins `GIT_CONFIG_GLOBAL=/dev/null`.
  Fix: set `GIT_CONFIG_GLOBAL=/dev/null` and `core.attributesFile=/dev/null` in both
  runners, pass the identity by `-c` (read once at the start), and let one test world
  leave the variable unset.
- **ADV12-2: the digest's baseline is re-read at every stage, so config written between
  rows is trusted.** Ran (coordinator): a `filter.fix.smudge` in the root's config before
  stage, plus the branch's `.gitattributes`. The gate was green, the merge merged, and
  `main:a.txt = "a1 a.txt"`. Two writers can put it there. One is the previous row's
  merged verify line: `MergeIfGreen` checks the digest before `mergedVerify`
  (`gate.go:808`) and never after, and that line runs branch code in a linked worktree
  of the root. The other is an escaped daemon (ADV11-8). Fix: bind the digest once per
  root per process (like the merges in memory), check it again after `mergedVerify`, and
  refuse (unrun) any program-valued key in the local or worktree config (`filter.*`,
  `merge.*.driver`, `diff.*.textconv|command`, `core.sshCommand`, `include*`).
- **ADV12-3: the pid file refuses ancestors only (a fork-storm door).** `PidFile`
  (`night/pidfile.go:96-135`) keeps a live holder that is not an ancestor. Ran: a direct
  child is refused (`nested: relay N is live`), but the same child double-forked,
  `( (sleep 1; exec child) & )`, is held and not refused. The hook allows
  `cd cmd/relay && go build -o /tmp/r . && /tmp/r run`, `go build -o /tmp/r ./cmd/rela?`
  and `go install ./cmd/...`. `env -i` drops the env guard. The nested night reads the
  worktree's own queue and spawns claude children. Fix: refuse any live holder, and let
  a second concurrent night start only with an explicit owner flag.

## Medium

- **ADV12-4: ANSI-C quoting blinds the guards and the allowlist.** `guards.go:565-572` and
  `packet.go:218-227` read `$'\''` as an unclosed quote. Ran in a sitting: `echo $'\'' ;
  git push origin main` and `echo $'\'' ; relay run` were allowed, and
  `Unbounded("echo $'\\'' ; make check")` returned `""`. `sh -c` and `zsh -fc` both run
  the second command. Fix: parse `$'…'` with its escapes, or refuse `$'`.
- **ADV12-5: arithmetic `<<` is read as a heredoc** (`guards.go:594-601`). Ran:
  `(( 1<<2 ))⏎git push origin main⏎2` was allowed, and zsh runs the push. Fix: open no
  heredoc inside `((…))` or `let`.
- **ADV12-6: a `-run` that matches every test passes** (`packet.go:429-447`, probes only
  `Testqzvq`). Ran: `-run 'Test[A-Z]'` passed and lists 20 of 20 tests; four packages with
  `'^(Test|Example)[A-Z0-9_]'` also passed, which is the suite. Fix: probe `TestZqzvq`,
  `Test_qzvq`, `Test9qzvq` and `ExampleZqzvq`, and allow one package only.
- **ADV12-7: the ledger is still trusted outside the night's own memory.**
  `StagedFromLedger` (`night/stage.go:470`, the last `stage` line wins) feeds `main_sha`,
  `state_digest`, `remote_urls` and `origin_refs` to the hand gate, `relay resume` and a
  restarted process (`expectedMain`, `gate.go:347`). `DepSatisfied` (`night/plan.go:415`)
  takes any green line. A line the child appends becomes the hand gate's main, digest and
  mirror, or satisfies a dependency. The merge's clean check blocks the update-ref route
  (reviewer). Fix: a row read from the ledger binds main, the digest and the URLs afresh,
  and a dependency is satisfied only by git (`landed`).

## Low

- ADV12-8: the Write door misses the pid dir when `XDG_STATE_HOME` points elsewhere (ran:
  `Write <home>/.local/state/agent-relay/relay.pid` was allowed), a `/private/tmp`
  spelling of a `/tmp` state root, and `~/`. Truncating the pid file empties the refusal.
- ADV12-9: the account stop: `API Error: 500 … authentication …` stops as `auth`, and an
  `API Error: 429` that does not start its line is missed (`account.go:55-68`). `Credit
  balance is too low` stops nothing.
- ADV12-10: hygiene. `gitStrict` turns a stderr warning into unrun, and child-written
  global config can cause one (a wrong red; ADV12-1's fix closes it). The owner's session
  is denied `rsync` with the text "a sitting never touches a device". The digest omits
  `objects/info/alternates`. The allowlist admits lines that never end (`cat /dev/zero`),
  bounded only by the line timeout.

## TLA gaps

The pid file's holder rule (ancestors only) is not modelled: `NoNestedSpawn` covers the
env guard, so ADV12-3 is outside TLC. The git config, as an input to the gate, is not
modelled either, and neither is the digest's per-stage baseline (ADV12-2).

## Verdict

**No: the relay is not ready to carry example-app.** ADV12-1, -2 and -3 decide it. On the
question the orchestrator asked: yes, the gate and the merge still read something the
child can write. That is the global git config (ADV12-1) and repo config written between
rows (ADV12-2). Both put a file that main's own check rejects onto main with a green gate,
and ADV12-1 also blinds the remote compare that backs the push guard. ADV12-3 reopens
the fork-storm door. The door is narrower than it was at ADV11. The merges no longer
read the ledger, main is bound for every row, the common hooks and config are digested,
the commit is tree-checked, and the push word holds as ruled. What remains has one root
cause, program-valued git config, plus one holder rule. Fix before example-app: ADV12-1 to 3,
and 4 to 7 in the same packet, since each is a few lines. These can wait: 8 to 10, and
ADV11-8, -13, -15 and -18 to -20.

## The three tests to add first

1. `TestAR21_0_GlobalConfigIgnored`: a gate world with `GIT_CONFIG_GLOBAL` unset and
   `$HOME/.gitconfig` holding a smudge filter and an `insteadOf`. The broken branch is not
   green, and a push by URL reads `origin_same=false`.
2. `TestAR21_1_ConfigBetweenRows`: a filter in the root's config before stage, and a
   merged verify line that writes one. Each is refused, and nothing merges.
3. `TestAR21_2_DetachedNestedRelay`: with a live holder, a double-forked child's
   `PidFile` returns `NestedError`.
