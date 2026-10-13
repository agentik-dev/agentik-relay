# Packet AR34: a stop said during the measure is kept

| item | value |
|---|---|
| from | `design/relay-r1.md` §9 (`stop` / `kill`), §15 (the measure's budget); the observation below |
| pin | agent-relay `main` at `3a21f83` plus this packet's own commit; worktree `.claude/worktrees/ar34-stop-in-measure`, branch `ar34-stop-in-measure` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, no live `claude`, fakes for the clock where time matters; no test sleeps more than 3 s in all. `go test ./...` only inside `make check`, in the foreground. No `relay run`, no `relay plan` against the account's state. No `make tla` or `make e2e`. Never the account's tmux socket |
| may touch | `internal/night/`, `internal/relay/`, `internal/effects/`, `cmd/relay/`, `design/relay-r1.md`, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR34_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR34_'` reads 4; `go test ./cmd/relay -run '^TestAR34_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR34_'` reads 1 |
| budget | 150 turns, 150k context, `claude-opus-5-5`; one commit per act (1 and 2); `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

§9: "`stop` / `kill [<row>]` | writes the night's control file (`stop`, or `kill <row>`), read by the loop after every event". `internal/night/night.go:398-399`, at the night's open: `control := ControlPath(opts.State, n.ID)` then `os.Remove(control)`.

What was seen on 2026-09-29. A night ran `relay run --rows r --measure-seconds 3000`, and its plan measured the baseline for about ten minutes; the trap had made the night's directory by then (`trap.go:85-100`). At 21:39:12, during the measure, the owner's session ran `relay kill r` and `relay stop`, and both printed `written; the loop reads it after its next event`. At 21:40:38 the night opened, removed the control file unread, and spawned `r` at 21:40:40. Neither command took effect, and the owner had to wait for the row. The measure itself never reads the control file, so a stop said at its start waits out the whole budget (up to `--measure-seconds`).

## 2. Rulings

Standing rules. Work in your worktree on branch `ar34-stop-in-measure` off `main`. The charter is `history/sittings/2026-09-30-1-ar34.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it in the charter). `DECISIONS.md` stays as it is; a fork this packet does not settle goes in the charter with the choice made.

1. **The open keeps what was said during the plan.** A control file whose modification time is at or after this run's start (`opts.Started`, or the plan's start; the one the run already holds) was written for this night: the open reads it with `ReadControl` before its first spawn, instead of removing it. `stop` stops the night `owner-stop`; any `kill` stops it `owner-kill`, because a kill already stops the whole night (NICE_TO_HAVE's line on `kill <row>`). No row spawns. Each row is skipped with `the night stopped: <reason>`, never red or failed, so the day's refusal of a red row (entry 8) does not fire on a row that never ran. A control file older than this run's start is a leftover, and is removed as today. Tests: `TestAR34_1_StopBeforeOpenIsKept` (a `stop` written after the start, before open: `stopped:owner-stop`, no spawn, rows skipped); `TestAR34_1_KillBeforeOpenIsKept` (the same for `kill r`: `stopped:owner-kill`, `r` skipped, not failed); `TestAR34_1_OldControlIsRemoved` (a file older than the start: removed, the night runs).
2. **A stop ends the measure.** While the plan measures under `run` (not under `plan`), it reads the night's control file every 2 s on `Clock` (injectable). On `stop` or `kill`, the budget counts as spent, as the deadline would, and every live measured group gets SIGTERM, then SIGKILL after `KillGrace`, the path the trap already takes (`trap.go`, `effects.Groups`). Each line not yet run, or cut, reads `unrun: owner-stop`, and the plan returns. Then act 1's open stops the night. The ledger's `plan` lines and `status.json` read as a plan cut by the owner, and a `measure-stopped` line names who stopped it and how many lines went unrun. Tests: `TestAR34_2_StopCutsTheMeasure` (night; a fake line that would run for minutes on the fake clock; a `stop` written at its start ends the plan within one poll, its group is signalled, the night stops `owner-stop` with no spawn); `TestAR34_2_StopVerbSaysWhenNotOpen` (cmd/relay; `relay stop` against a night that has a directory and no `status.json` prints `the night is still measuring: it stops within a few seconds, before any row starts`, not `after its next event`).

After the acts: §9's `stop` / `kill` row gains one sentence (said during the measure: the measure ends and no row starts), and §5's events name `measure-stopped`.

## 3. Interfaces (signatures, not prose)

`NightOptions` or the plan's `Options` gains what act 2's poll needs: the control path, a `Poll time.Duration` (default 2 s; tests pass a fake clock), and the group registry the trap uses. `boundShell` gains a way to be stopped from outside (for example `stop(reason string)`, safe from another goroutine) that marks the budget spent and records the reason for the lines it leaves unrun. `ReadControl` keeps its signature; the paused branch `ar32c-words-push-view` changes it, so do not.

## 4. Findings that name this row

On `main` at `3a21f83`: `internal/night/night.go:382` (the night starts), `:394-399` (the dir, the control path, the remove), `:948` (the loop's `ReadControl`); `internal/night/control.go:12-40`; `internal/night/plan.go:197-200` (the night's name), `:306-317` (the bound shell and its budget), `:795-822` (`baseline`); `internal/night/bound.go:254-330` (`boundShell`, `bound`, `remaining`, `Run`); `internal/night/trap.go:27-100` (the trap, its groups, the dir); `internal/relay/night.go:20-55` (`owner-stop`, `owner-kill`); `cmd/relay/main.go:1330-1350` (`controlVerb`).

## 5. Forbidden reads

Other repos; the real state root, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: per act the red test on `main`; each fork decided; what was left. Last line: branch, head sha.
