# Packet AR4: the fixes after the chain merged

| item | value |
|---|---|
| from | `design/relay-r1.md` §1, §3, §5, §6, §9; `design/review-ar2.md` AR2-10, AR2-11, AR2-19, AR2-21 and its state-directory observation; the owner's rulings of 2026-09-22 16:30 (items 5, 6, 7, 11, 13, 14, 15, 17 of the brief), restated in §2. The builder reads THIS packet and those sections; never the brief, never a ledger |
| pin | agent-relay `main` at its head (cdf6bdb when this was cut; the relay cuts `ar4-fixes` off it); worktree `.claude/worktrees/ar4-fixes` |
| wall | this repo only; no push, `--amend`, rebase, device or network; the real spawn line is never run; `bin/relay` never runs without `XDG_STATE_HOME` set to a fresh temp directory, in tests or by hand; `README.md` and `design/readme/` are another row's |
| may touch | `design/relay-r1.md` (one sentence per act in §1, §3, §5, §6, §9), `internal/relay/types.go`, `internal/relay/machine.go`, `internal/relay/gate.go`, `internal/relay/machine_test.go`, `internal/night/plan.go`, `internal/night/plan_test.go`, `internal/night/run.go`, `internal/night/run_test.go`, `internal/night/night.go`, `internal/night/night_test.go`, `internal/night/status.go`, `internal/night/status_test.go`, `internal/night/report.go`, `internal/night/report_test.go`, `internal/night/stage.go`, `internal/night/testdata/json-keys.golden`, `internal/gate/gate.go`, `internal/gate/gate_test.go`, `internal/effects/effects.go`, `internal/effects/effects_test.go`, `internal/effects/git.go`, `internal/effects/git_test.go`, `internal/effects/fake.go`, `internal/effects/ledger.go`, `internal/effects/ledger_test.go`, `internal/queue/queue.go`, `cmd/relay/main.go`, `cmd/relay/main_test.go`, `plugin/commands/resume.md`, `plugin/commands/plan.md`, `plugin/commands/gate.md`, `plugin/commands/status.md`. Nothing else; the charter is admitted by §6 (act 3) |
| acceptance | `make check; echo $?` reads 0; `go test -race ./... >/dev/null 2>&1; echo $?` reads 0; `go test ./... -run 'TestAR4_' -v 2>&1 \| grep -c '^--- PASS'` reads 8; `grep -rl GitWriter internal cmd \| wc -l` reads 0; `grep -c '[A-Z]' internal/night/testdata/json-keys.golden` reads 0; `grep -c '^status rows.\[\]worktree' internal/night/testdata/json-keys.golden` reads 1; `grep -c 'agent-relay:plan' plugin/commands/resume.md` reads 0; `grep -c 'max-turns 1 --version' cmd/relay/main.go` reads 1 or more; `go build -o bin/relay ./cmd/relay && XDG_STATE_HOME=$(mktemp -d) bin/relay doctor 2>&1 \| grep -c '^FAIL claude'` reads 0; `grep -c "ancestor of the row's base" design/relay-r1.md` reads 0; `grep -c 'history/sittings' design/relay-r1.md` reads 1 or more; `grep -l '^func TestMain' internal/night/*_test.go cmd/relay/*_test.go internal/effects/*_test.go internal/gate/*_test.go \| wc -l` reads 4; `git status --short \| wc -l` reads 0 after the last commit |
| budget | 120 turns, 130k context, `claude-opus-5-5`; `make check` carries `-race`; one commit per act (nine acts, §3); checkpoint at 80%; never push, amend or rebase; the charter is the first free `history/sittings/2026-09-22-<n>.md` on the branch |

## 1. What the source says (quoted)

§3: a dependency is satisfied "when branch `<id>` is an ancestor of the row's base". §6: the verify line is "green, or same-as-main, never failed on its own"; step 7: "`merge --no-ff --no-commit`, the verify line, then `git commit`; on red, `git merge --abort`". §5: "The `sitting` line stays a superset of the stand-in's"; the state directory is "`${XDG_STATE_HOME:-~/.local/state}/agent-relay`". §9: "`--help` lists every flag §7 uses"; "unknown is never a pass". AR2-10: "`IsAncestor(a, a)` is true". AR2-19: "which terminal state it maps to is the machine's to name". The observation: "loadPlan read the real state directory's ledger".

## 2. Rulings

The owner's standing word: decide inside the design, record each choice in the charter, escalate at most ONE real fork. One ruling per act:

1. A dependency is satisfied by a green ledger line, by branch `<id>` being an ancestor of `main` in the root, or by being in this queue and green tonight; never by ancestry of the row's own base. §3's sentence changes to say so.
2. `doctor` probes `claude --max-turns 1 --version` and reads exit 0 (hidden from `--help` in 2.1.280, accepted); the `--help` scan goes; §9's row says so. `SpawnFlags` goes when nothing else reads it.
3. §6 rules that every touch set admits `history/sittings/*.md` and `history/sessions/NEXT` without listing them; `gate.RulesPaths` applies it and `TestGateGreen` proves it already: this act is the sentence, no code.
4. An unrun gate is red: the sitting machine gains `gate-unrun` (`exited` → `red`, reason `gate-unrun`), `RunRow` steps it, the night moves on, `status` shows `red: gate-unrun`, the resume line prints; §1's table gains the row.
5. D22: `plan` runs the repo's verify line (`VerifyLine`) once on `main` and records it (`Plan.Verify`, `--json` key `verify`, the `plan` ledger line); after `merge --no-commit` the verify is green (exit 0) or same-as-main (exit and last line equal the baseline's), else abort. No baseline (a hand gate): green only.
6. `effects.Git` absorbs `GitWriter`'s eleven methods; `Writer` goes; `FakeGit` and `OSGit` already implement them; `TestOSGitWriter` is renamed.
7. The `sitting` line carries every key of the stand-in's: `row`, `repo`, `cwd`, `mode`, `branch`, `base`, `prompt_hash`, `rules_hash`, `model`, `max_turns`, `budget_usd`, `head_before`, `head_after`, `session_id`, `turns`, `cost_usd`, `duration_ms`, `wall_s`, `child_exit`, `is_error`, `denials`, `gate` (an object with `result` and `verify`), `stop`, `next_after`. `repo` the root, `cwd` the worktree, `mode` `worktree`, `stop` the kill reason or empty, `next_after` empty (the loop knows it). Readers of `gate` take older lines' string too.
8. `RowStatus` gains `worktree`; every `--json` key is snake_case (`relay.Check`, `relay.Gate`, `queue.Problem`, `gate --json`); the golden is retaken with `gate` added; the plugin commands read the new names; `/agent-relay:resume` prints the worktree from `status --json`.
9. The state root is `XDG_STATE_HOME` (§5), the one variable: a `TestMain` in `internal/night` (extend it), `internal/gate`, `internal/effects` and `cmd/relay` sets it to a temp directory before any test; `night.LockDir` follows it.

## 3. Interfaces (signatures, not prose)

- `plan.go`: `satisfied(g, root, q, lines, id string)` (no `base`); `Plan.Verify *relay.Check` with `json:"verify"`.
- `cmd/relay/main.go`: `var claudeProbe = func(path string) error` running `--max-turns 1 --version`; the check detail `<path>; accepts --max-turns 1 --version`.
- `relay`: `GateUnrun Event = "gate-unrun"`; `GateUnrunReason StopReason = "gate-unrun"`; `Check` and `Gate` fields tagged snake_case.
- `gate.go`: `MergeIfGreen(fx, root, g, branch, verify string, baseline *relay.Check)`; `night.NightOptions.Merge` passes the plan's.
- `effects.go`: `Git` with the eleven write methods folded in; `git.go` loses `GitWriter` and `Writer`.
- `status.go`: `RowStatus.Worktree string` `json:"worktree"`, set when the row is planned.
- Tests, eight: `TestAR4_DepNeverSatisfiedByOwnBase` (plan_test); `TestAR4_DoctorProbesMaxTurns` (main_test); `TestAR4_UnrunGateIsRed` (night_test: a gate returning `Unrun` ends the night `queue-empty`, the row red, its resume line printed); `TestAR4_MergeVerifyAsMain` (gate_test: red on main with the same exit and last line merges, a different last line aborts); `TestAR4_SittingLineKeys` (run_test: the 24 keys, `gate.verify` inside); `TestAR4_StatusRowWorktree` (status_test); `TestAR4_JSONKeysSnake` (main_test: no capital in the golden); `TestAR4_StateDirIsTemp` (night_test: `effects.StateDir(os.Getenv)` and `LockDir()` lie under `os.TempDir()`).

## 4. Findings that name this row

Baseline on `main` at cdf6bdb, 2026-09-22, temp `XDG_STATE_HOME`: `make check` 0; `go test -race` 0; `TestAR4_` reads 0; `GitWriter` in 4 files; the golden has 9 capitalised lines (`queue.Problem`'s and `relay.Check`'s fields) and no `status rows.[]worktree`; `resume.md` names `/agent-relay:plan` once; the probe absent; `doctor` reads 1 (`--help lacks --max-turns`) while the probe exits 0 here; "ancestor of the row's base" once; `history/sittings` absent from relay-r1; one `TestMain` (`internal/night/run_test.go`). `effects.Writer(` is called in `internal/gate/gate.go`, `internal/night/stage.go` and `cmd/relay/main.go`.

## 5. Forbidden reads

Every other repo; `~/.claude/projects/`; `~/.local/state/agent-relay/`; any ledger outside `t.TempDir()`; the brief; `design/relay-r1.md` beyond the sections named; `design/review-ar2.md` beyond the entries named; `design/ux-review.md`; `.claude/worktrees/`; any file over 8 KB whole unless in `may touch`.

## 6. Report (capped: one table, at most five bullets, under 1,500 words)

The table: each acceptance line, its exit and its last output line on the branch. Bullets: every choice made inside the design with the relay-r1 sentence it changed; the golden's diff in one line; the one open fork for the owner, if any, with its consumer; what was left undone. The last line: the branch name and its head sha.
