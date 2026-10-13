# Packet AR35: five timing tests that hold on a loaded machine

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md` (the gate's `make check` flaked under load); the observation below |
| pin | agent-relay `main` at `7bd029d` plus this packet's own commit; worktree `.claude/worktrees/ar35-load-tolerant-tests`, branch `ar35-load-tolerant-tests` |
| wall | this repo only; test files only, plus `NICE_TO_HAVE.md` and the charter. No push, `--amend`, rebase, device, ssh or network. No production code changes: if a test can only be fixed by changing the code under test, stop and name it in the charter. Never start CPU burners to make load; the machine is shared. `go test ./...` only inside `make check`, in the foreground |
| may touch | `internal/night/*_test.go`, `internal/effects/*_test.go`, `internal/night/testdata/`, `internal/effects/testdata/`, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run 'TestCeilingKill\|TestAR5_OwnerKillEscalates\|TestAR28_3_CostUnread' -race -count=10 2>&1 \| grep -c '^ok'` reads 1; `go test ./internal/effects -run 'TestAR10_9_WaitReturnsWithinASecond\|TestAR20_3_LsRemoteTimesOut' -race -count=10 2>&1 \| grep -c '^ok'` reads 1 |
| budget | 100 turns, 120k context, `claude-opus-5-5`; one commit per test fixed; `make check` once at the end, in the foreground |
| verify | `make check` |

## 1. What the source says (quoted)

`NICE_TO_HAVE.md`: "The gate's `make check` flaked twice under load; re-run it before acting on a red. In ar30's build round, `TestCeilingKill` and `TestAR28_3_CostUnread` each missed a timing limit once on a loaded machine and passed alone with `-race`, 3 of 3."

What was seen on 2026-09-29, at a load of 8 to 17 on 16 cores. Every full `make check` in three merges failed on one or two of these five tests, and each passed alone three times out of three:
- `TestCeilingKill`: "the kill took 3.19s" against a 2 s bound;
- `TestAR10_9_WaitReturnsWithinASecond`: failed after 0.42 s;
- `TestAR20_3_LsRemoteTimesOut`: "the fake ssh never ran in 3 attempts";
- `TestAR28_3_CostUnread` and `TestAR5_OwnerKillEscalates`: timing misses under load.

Each red costs a rerun, and teaches everyone to ignore a red.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar35-load-tolerant-tests` off `main`. The charter is `history/sittings/2026-09-30-2-ar35.md` in doclint's five-part shape, opened first, closed before the last commit, no home path. `make check` is 0 at the last commit. `DECISIONS.md` stays as it is. Red-on-main does not apply to this packet: a flake cannot be made to fail on demand. Instead, the charter records the acceptance lines' results on `main` before any change; they may pass by chance.

1. **Keep each test's intent, and write it down.** For each of the five, the charter names in one line what the test proves: for example, that the ceiling kill ends the child before its work would finish, or that `Wait` returns promptly.
2. **A bound that guards against a hang is not a latency claim.** Where a wall-clock bound only proves that something ends, move it to at least ten times the expected time. Where the scenario allows, prove the outcome instead of the time. For example, the killed child wrote fewer lines than it would have written, or the fake child was made slow enough that finishing is impossible inside the bound.
3. **A bound that is the claim** (`WaitReturnsWithinASecond`) keeps its number. The test tries up to three times, passes on the first attempt that meets it, and logs each miss with `t.Logf`. A latency claim holds if it holds once in three tries on a loaded machine; one that never holds is still red.
4. **Retries that wait for a fake to start** (`LsRemoteTimesOut`'s "the fake ssh never ran") wait for the fake's own signal, a file or a pipe it writes, with a generous deadline, not a fixed count of short attempts.
5. **Do not weaken what a test proves.** No `t.Skip` under load, and no reading of the load average. Do not delete an assertion. Do not change `KillGrace`, `LsRemoteTimeout` or any production value except where the test already sets it.

After the fixes, `NICE_TO_HAVE.md`'s line on the flaky gate is replaced by one line that names this packet and any test left as it was.

## 3. Interfaces (signatures, not prose)

None; tests only. A shared helper, if two tests need one, lives in a `_test.go` file of its package, for example `func eventually(t *testing.T, tries int, f func() error)`.

## 4. Findings that name this row

On `main` at `7bd029d`: `internal/night/run_test.go:277-310` (`TestCeilingKill`, the 2 s bound at `:298`); `internal/effects/ar10_test.go:211-233` (`TestAR10_9_WaitReturnsWithinASecond`, the 1 s bound at `:231`); `internal/effects/ar20_3_test.go:17-70` (`TestAR20_3_LsRemoteTimesOut`, the attempts at `:40-55`); `internal/night/night_test.go:464-492` (`TestAR5_OwnerKillEscalates`, the 5 s bound at `:490`); `internal/night/ar28_test.go:150-` (`TestAR28_3_CostUnread`).

## 5. Forbidden reads

Other repos; the real state root, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line, on `main` and at the head. Bullets: per test its intent and what changed. Last line: branch, head sha.
