# Packet AR94: AR59's two cut tests prove the cut by its outcome, not by a 3 s wall

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`, "AR93's build (2026-10-16): what it leaves", its S3 (line 719), and "Observer, 2026-10-11", its first bullet (line 442); `DECISIONS.md` entries 2 and 10 (§1); `design/packets/ar35-load-tolerant-tests.md` §2; `design/packets/ar93-load-flakes.md` §2; `design/relay-r1.md` §11; the survey `research/synthesis/tools-relay.md` in agentik-research, §1.3; the code at e2bbf7c |
| pin | `main` at e2bbf7c plus this packet's own commit; a linked worktree, branch `ar94-gate-cut-tests` |
| wall | AR93's: this repo only; no push, `--amend`, rebase, device, ssh or network; no `relay` binary by hand or outside `make`'s harness; no `relay` or `agentik relay` verb on the account's state; never `claude`. No `make test`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/gate`, `gofmt -l internal`, `make doclint`, `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. One `make check`, last, started only once `ps -axo command \| grep -E '(^\|/)make( \|$)' \| grep -v grep` prints nothing, and waited for. Never spell the Go name of the local zone. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/gate/ar59_test.go` (`TestAR59_2_GateUsesLimit`, `TestAR59_3_MergeNamesCut`, their two helpers and one import only), `internal/gate/ar94_test.go` (new), `design/relay-r1.md` (§11's test list only), `NICE_TO_HAVE.md` (two marks; one new section only under ruling 6), `history/sittings/*.md`, `history/sessions/NEXT`. Nothing else |
| acceptance | `go test ./internal/gate -run '^TestAR94_' -race -count=3 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR94_'` reads 6 (main: 0); `go test ./internal/gate -run '^TestAR59_(2_GateUsesLimit\|3_MergeNamesCut)' -race -count=5 -timeout 300s 2>&1 \| grep -c -E '^ok '` reads 1 (main: 1); `go test ./internal/gate -run '^TestAR(59_\|78_\|94_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 (main: 1) |
| budget | 60 turns, 180k context, `claude-opus-5-5`; three acts, one commit each; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says (quoted)

NICE (S3, 2026-10-16): "AR93's fix-round `make check` (2026-10-16 23:16) read `TestAR59_2_GateUsesLimit`: `the cut gate took 3.4697095s: the line ran past its limit` (`internal/gate/ar59_test.go:29`), … load 4 rising to 21 on 16 cores; the first is "Observer, 2026-10-11" (3.047 s). A wall deadline on a 1 s cut, not an order race". The hq sitting saw `TestAR59_3_MergeNamesCut` read 3.9 s over its 1 s cut once at load 23 (2026-10-16). Alone each passes. AR35's ruling 2: "A bound that guards against a hang is not a latency claim. … Where the scenario allows, prove the outcome instead of the time."; ruling 5: "No `t.Skip` under load, and no reading of the load average. Do not delete an assertion." relay-r1 §11 (AR78's fix round) "dropped a 1 s bound the merge's own git outran under load".

`DECISIONS.md`, verbatim. Entry 10: "A repo's relay config may carry its own verify limit; when it is unset, the limit stays 900 s. `agentik relay doctor --verify` measures the repo's verify line and suggests a limit. A verify line cut at its limit reads red with a reason that says the line was cut, never with an empty reason." Entry 2, its last sentence: "Every packet follows relay-r1 §2, its writer's appended notes included: under 9,216 bytes, six sections, and acceptance on the allowlist." The survey (§1.3): "The fix waits on a condition, not a longer sleep".

The code at e2bbf7c. Each test bounds a whole call at 3 s (`ar59_test.go:22-28` over `Run`, `:114-119` over `MergeIfGreen`), but the cut is timed on the line alone (`gate.go:974`, `:1036`, just before `effects.RunLogged`; `effects.Cut`, `effects.go:175`), while the call also clones a fresh tree (`Fresh`, `gate.go:918`, from `:452` and `:1022`), overlays, merges and writes a tree. The cut kills the whole group: `Setsid` and a `Cancel` that SIGKILLs `-pid` (`shell.go:156-157`), then both sweeps (`:195`, `:201`), and 124 at the deadline (`:224`). The line runs through `sh -c` in the fresh tree, verbatim (`night/plan.go:862`). `w.fx.Git` is `effects.OSGit{}` (`gate_test.go:80`), value receivers: a wrapper can embed it.

Measured. Alone on the pin, each test `-race -count=10` 10 of 10 (load 3.5 to 4.8). Under `-race` at load 4 to 5, `Run` with `true` took 0.63 to 0.90 s, the cut `Run` 1.73 to 1.74 s, the cut `MergeIfGreen` 1.55 to 2.60 s: up to 1.6 s of git beside the 1 s cut. Forced (each `Clone` 2 s late, the old bound): the cut gate took 3.64 to 3.67 s and the merge's verify ran 3.58 to 3.68 s, red 3 of 3 each (load 5 to 9); with §2's device 3 of 3 green each, twice (62 s a run, load 3 to 9). With the shell's `Cancel` a no-op, both old names read red on the mark alone. With the device the old names read 20 of 20 at `-count=10` (129 s, load 5 to 7), about 6 s a run, hence line 2's `-count=5`; line 3 reads 1 on both (45 s); `go vet` 0, `gofmt -l` empty.

## 2. Rulings

Standing rules. The charter, `history/sittings/<date>-<n>-ar94.md`, opens at act 1 and closes before the last commit; no home path; it records acceptance line 2 at its open. Each act's first test is red on the pin, its red line in the commit message. `DECISIONS.md` stays as it is. AR35's ruling 5 binds; no product code or production value changes.

1. **The device** (AR35's ruling 2). The 3 s bound only proved the cut, so each test proves the outcome: the cut line becomes `sleep 3 && touch '<mark>'`, `mark` under `t.TempDir()`, and `ar94Absent` asserts the mark absent no earlier than 4 s after the call returned. The line began by its deadline, before the return, so its sleep was over by the return plus 3 s; a shell the cut left alive has touched the mark by then (a 1 s margin). The cut's own signals stay asserted (red, exit 124, `Cut` 1, `VerifySeconds` 1; the refusal's text, the `merge` line `verify-cut`, the `MergeRefused`, the root unmoved and clean). The 3 s bound becomes a hang guard, `ar94Bound` 30 s (AR78's). AR59_2's second, unlimited `Run` stays and its mark is checked after it.
2. **Act 1, the gate.** `ar59GateCut(t, slow)` holds AR59_2's body; the old name calls it at 0, `TestAR94_1_GateCutUnderSlowGit` at 2 s. `ar94SlowClone` (`effects.OSGit` embedded) sleeps `slow` in each `Clone`, inside the call: a forced call takes 2 s plus the 1 s cut, 3 s or more by construction, and `ar94Forced` fails one under 3 s so the forcing cannot lapse. Red first: the helper with the old bound.
3. **Act 2, the merge.** `ar59MergeCut(t, slow)` and `TestAR94_2_MergeCutUnderSlowGit` likewise over `MergeIfGreen`; the helper's `fx` serves `Gather` too.
4. **The shards stay**, no `-p` cap (settled 2026-10-16): the red was a wall bound inside one test, which ruling 1 removes; NICE's note that AR93 §6's fork condition reads on it is answered here.
5. **What stays**: `TestAR59_3_CutReason` (`sleep 3` under 1 s, no wall bound) is unchanged; no other `internal/gate` test bounds a call's wall (`time.Since`, at e2bbf7c).
6. **Act 3, the record.** relay-r1 §11: the AR59_2 line's "in under 3 s" and the AR59_3 line gain their device (AR94): the touch after the sleep never lands, read 4 s after the return, the hang guard 30 s; after `TestAR93_2`'s line, the two `TestAR94_` lines. NICE: the S3 at line 719 and the gate test's sentence at line 442 gain `(AR94)`; a section "AR94's build (2026-10-17): what it leaves" only if the build leaves something.

## 3. Interfaces (signatures, not prose)

Test-only. `gate` tests: `ar94SlowClone{effects.OSGit; slow time.Duration}` and its `Clone`, `const ar94Bound`, `ar94Absent(t, path string, back time.Time)`, `ar94Forced(t, slow, took time.Duration)`, `ar59GateCut(t, slow)`, `ar59MergeCut(t, slow)`, `TestAR94_1_GateCutUnderSlowGit`, `TestAR94_2_MergeCutUnderSlowGit`. No product code, `--json` key, flag, event or exported name.

## 4. Findings that name this row

NICE "AR93's build (2026-10-16): what it leaves", its S3 (the second sighting); "Observer, 2026-10-11", its first bullet (the first sighting).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo (§1 quotes the survey).

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count; the exits of `go vet`, `gofmt -l`, `make doclint` and `make check`; `make hygiene`'s last line. Bullets: each act's red line; line 2 at the open and at the end; choices beyond §2; anything filed. Last line: branch, head sha. Fork for the owner: none; the device proves the outcome, and the shards are settled.
