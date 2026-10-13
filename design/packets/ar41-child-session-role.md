# Packet AR41: a relay child is metered in its role

| item | value |
|---|---|
| from | `design/relay-r1.md` §7 (the child's environment); the portfolio ledger's entry 17 ("Builders, helpers and relay children stay on the 200k window … The owner may put a repo's relay rows on Fable 5.1 with a wider budget, set in that repo's own queue and session config"); claude-code-session `claude_session/hooks.py:186-196` (`launch_profile` reads `CLAUDE_SESSION_PROFILE`) and `resolver.py:163-167` (a project layer outranks the role); the observation below |
| pin | agent-relay `main` at 557d989 plus this packet's own commit; worktree `.claude/worktrees/ar41-child-session-role`, branch `ar41-child-session-role` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state |
| may touch | `internal/effects/spawn.go`, `internal/effects/*_test.go`, `internal/night/run.go`, `internal/night/*_test.go`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/effects -run '^TestAR41_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR41_'` reads 1; `go test ./internal/night -run '^TestAR41_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR41_'` reads 1 |
| budget | 60 turns, 80k context, `claude-opus-5-5`; one commit for the act; `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`internal/effects/spawn.go:74-92`: `ChildEnv` is "DropSitting's drop (every CLAUDE* variable …)", then `RELAY_ROW`, `RELAY_MEASURE_DEPTH=1`, `CLAUDE_SESSION_DEPTH=1` and `CLAUDE_SESSION_MAX_TURNS=<turns>`. It sets no `CLAUDE_SESSION_PROFILE`. claude-code-session's `launch_profile` returns `CLAUDE_SESSION_PROFILE` (or `CLAUDE_SESSION_CONFIG`), else `None`, and `None` resolves the default profile, `frontier`.

What was seen on 2026-09-30 at 12:40. example-app's l1b child (night 2026-09-30-4) wrote `request.json` with `"profile": null` and resolved `profile: frontier`, `posture: signal`. Its meter read 400k on the 1M window only because example-app's project layer (`.claude/claude-code-session/config.yaml`, `ceilings_1m: {context: 400000}`) sets that for every example-app session. A relay child is a builder, whose profile has `posture: deny`. claude-code-session's NICE_TO_HAVE asks for this export ("The relay should export CLAUDE_SESSION_PROFILE=builder to the children it spawns").

## 2. Rulings

Standing rules. Work in your worktree on branch `ar41-child-session-role` off `main`. The charter is `history/sittings/2026-09-30-8-ar41.md` in doclint's five-part shape, opened at the act, closed before the last commit, no home path. The act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is.

1. **The child carries its role.** `ChildEnv` gains a `profile string` argument and, after the `CLAUDE*` drop, adds `CLAUDE_SESSION_PROFILE=<profile>` (a new `SessionProfileVar` beside `SessionDepthVar`), so an inherited value never passes. `profile == ""` adds none. `run.go`'s spawn passes `orchestrator` for an orchestrator sitting and `builder` for every other row. Take the orchestrator test from the plan's own marker, the one `run.go:226` names (an orchestrator sitting's `RowPlan.Row` is set, AR32A act 1). Confirm it against `plan.go:383` and `orchestrator.go:164`, and name the marker you used in the charter. Tests: `TestAR41_1_ChildEnvCarriesProfile` (effects) gives `ChildEnv` an environ holding `CLAUDE_SESSION_PROFILE=orchestrator` and the profile `builder`, and expects exactly one `CLAUDE_SESSION_PROFILE=builder`; with `""` it expects none. `TestAR41_1_RunPassesRole` (night) runs a one-row night and an orchestrator sitting through the fake spawner, and reads `CLAUDE_SESSION_PROFILE` in each spawn's env: `builder`, then `orchestrator`.

After the act, §7's environment list gains `CLAUDE_SESSION_PROFILE` in one sentence: the row's role, `builder` or `orchestrator`; a repo that gives its rows a wider budget sets it in its own session config, which outranks the role.

## 3. Interfaces (signatures, not prose)

`func ChildEnv(environ []string, row, profile string, turns int, remotes []string, bin string) []string`; `const SessionProfileVar = "CLAUDE_SESSION_PROFILE"`. Every caller and test of `ChildEnv` is updated.

## 4. Findings that name this row

`internal/effects/spawn.go:74-92` (`ChildEnv`), `:150-154` (the session vars); `internal/night/run.go:225-232` (the spawn); `internal/night/plan.go:380-386` and `internal/night/orchestrator.go:155-165` (where an orchestrator's spawn gets `WithAgent`).

## 5. Forbidden reads

Other repos but the two claude-code-session files named in `from`; the real state root and everything under the account's `~/.local/state`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: the red test on `main`; the orchestrator marker used; what was left. Last line: branch, head sha.
