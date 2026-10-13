# Packet AR5: the printed lines UX1 handed over, the kill, and the tests AR2 asked for

| item | value |
|---|---|
| from | `design/relay-r1.md` §7 (the spawn, the kill escalation, the live line), §9 (`run`, `report`, `resume`), §11 (what the end-to-end test asserts); `design/ux-review.md` `## Handover` (its four items); `design/review-ar2.md` AR2-12, AR2-16, AR2-18, AR2-20, AR2-22; the owner's rulings of 2026-09-22 16:30 on items 18 and 22 of the brief, restated in §2. The builder reads THIS packet and those sections; never the brief, never a ledger |
| pin | agent-relay branch `ar4-fixes` at its head (merged or not: the relay cuts `ar5-ux` off it); worktree `.claude/worktrees/ar5-ux` |
| wall | this repo only; no push, `--amend`, rebase, device or network; the real spawn line is never run (every night runs over the fake claude); `bin/relay` never runs without `XDG_STATE_HOME` set to a fresh temp directory; `README.md` and `design/readme/` are another row's |
| may touch | `design/relay-r1.md` (one sentence each in §7 and §9), `internal/relay/types.go`, `internal/stream/stream.go`, `internal/stream/stream_test.go`, `internal/night/plan.go`, `internal/night/plan_test.go`, `internal/night/night.go`, `internal/night/night_test.go`, `internal/night/run.go`, `internal/night/run_test.go`, `internal/night/report.go`, `internal/night/report_test.go`, `internal/night/status_test.go`, `internal/gate/gate_test.go`, `internal/effects/fake.go`, `internal/effects/testdata/fakeclaude/main.go`, `cmd/relay/main.go` (the `run --dry` spawn line only), `cmd/relay/main_test.go`. Nothing else; no `--json` key changes; the charter is admitted by §6 |
| acceptance | `make check; echo $?` reads 0; `go test -race ./... >/dev/null 2>&1; echo $?` reads 0; `go test ./... -run 'TestAR5_' -v 2>&1 \| grep -c '^--- PASS'` reads 10; `grep -h 'HasPrefix(l, "  spawn: ")' internal/night/status_test.go cmd/relay/main_test.go \| wc -l` reads 0; `grep -c '"; relay resume "' internal/night/night.go` reads 0; `grep -c 'stopped:origin-moved' internal/night/night_test.go` reads 1 or more; `grep -Ec 'MergeSHA\|merge-on-green' internal/night/night_test.go` reads 1 or more; `grep -c 'prompt_hash' internal/night/run_test.go` reads 1 or more; `grep -c 'KillGrace' internal/night/night.go` reads 1 or more; `git status --short \| wc -l` reads 0 after the last commit |
| budget | 120 turns, 130k context, `claude-opus-5-5`; `make check` carries `-race`; one commit per act (six acts, §3); checkpoint at 80%; never push, amend or rebase; the charter is the first free `history/sittings/2026-09-22-<n>.md` on the branch |

## 1. What the source says (quoted)

§7: "at 100% SIGTERM, thirty seconds, SIGKILL"; the ledger lines are "appended before and after every spawn"; the live line is "`turn N: <tool> "<summary>"; context 71k/130k; $1.20`". §9: `report` is "what stopped and the verbatim line"; every red or failed row's line "print[s] this exact line" (`relay resume <row>`). UX1's handover: "Wrap plan's and `run --dry`'s `spawn:` line ... under 100 columns and write home as `~`. Doing this means changing `TestPlanEqualsRun`"; "`relay r1: red; relay resume r1` should put `relay resume r1` on a line of its own, and `relay r1: running in <abs worktree>` should write home as `~`"; "The live log writes one line per stream event, so an assistant message with two tool uses repeats its line". AR2-18: "`relay kill` sends SIGTERM with no SIGKILL thirty seconds on". AR2-20: "the spawn line's append error is swallowed, the child spawns anyway". AR2-12, AR2-16, AR2-22: no test drives a night to `stopped:origin-moved`, reads the spawn line `RunRow` writes, or merges.

## 2. Rulings

The owner's standing word: decide inside the design, record each choice in the charter, escalate at most ONE real fork. One ruling per act:

1. The `spawn:` line of `plan` and `run --dry` wraps with ` \` continuations, every piece under 100 columns, home written `~` (a shell expands it); `TestPlanEqualsRun` joins the continuations and expands `~` before comparing; the exemptions in `TestPrintedLinesUnder100` and `cmd/relay`'s under-100 test go.
2. The report's `## Stopped` renders the `stop` line as `key: value` lines, keys sorted, `event` dropped, a list joined by `, `, no fence; `Stopped.Line` keeps its `--json` key; §9 loses "verbatim".
3. The night's per-row lines: `relay <row>: running in ~/...`; a red or failed row prints `relay <row>: <state> [reason]` and then `relay resume <row>` on its own line.
4. One live line per assistant message: `stream.Tally` gains `Fresh bool`, true when `Add` saw a new message id; `RunRow` writes the line on `Fresh` and on `result`, never on a repeat event.
5. The owner's kill escalates as the ceiling does: `held.kill` sends SIGTERM and, after `KillGrace`, SIGKILL unless the child has exited; §7 says "the owner's `kill` the same way". A failed `spawn` ledger append is a sitting failure: `RunRow` returns `failed` with the new reason `ledger-error` and spawns nothing.
6. The tests AR2 asked for, over the fake claude and `t.TempDir()` git repos, never the real state directory.

## 3. Interfaces (signatures, not prose)

- `plan.go`: `WrapSpawn(worktree string, argv []string, home string) []string` (the printed pieces); `Print` and `main.go`'s `run --dry` use it.
- `stream.go`: `Tally.Fresh bool`; `Line` unchanged.
- `night.go`: `held.kill()` with the `KillGrace` timer; the per-row lines as ruling 3.
- `relay/types.go`: `LedgerError StopReason = "ledger-error"`, a sitting failure, the night stops.
- `report.go`: `stopLines(line string) []string` (the `key: value` rendering).
- `fake.go`: `FakeLedger.FailOn string` (the event whose `Append` returns `Err`), beside `Err`.
- Tests, ten: `TestAR5_SpawnLineWrapped` (plan_test: every plan line under 100 columns, the pieces joined equal the argv); `TestAR5_StopLineKeyValue` (report_test); `TestAR5_NightLinesResumeAndTilde` (night_test); `TestAR5_OneLiveLinePerMessage` (run_test: `real.jsonl`'s two-tool message logs one line); `TestAR5_OwnerKillEscalates` (night_test: `FAKECLAUDE_IGNORE_TERM`, `KillGrace` short, the row `failed owner-kill` and the child gone); `TestAR5_SpawnLineAppendErrorStops` (run_test: `FailOn: "spawn"`, no spawn recorded, the sitting `failed ledger-error`); `TestAR5_OriginMovedStopsNight` (night_test: the remote refs change after stage, the night ends `stopped:origin-moved`); `TestAR5_MergeOnlyInMergeIfGreen` (gate_test: a source scan of `internal/` and `cmd/` finds `MergeNoCommit(` called in `MergeIfGreen` only); `TestAR5_SpawnLineHashes` (run_test: the `spawn` line `RunRow` wrote carries `prompt_hash` equal to the stdin's sha256 and `rules_hash` equal to `prompt`'s); `TestAR5_NightMerges` (night_test: a `merge-on-green` row on a temp repo whose root is on `main`; after the night `main`'s head is the `MergeSHA` in `status.json` and the `merge` ledger line).

## 4. Findings that name this row

Baseline on `main` at cdf6bdb, 2026-09-22: `make check` 0; `go test -race` 0; `TestAR5_` reads 0; the `spawn:` exemption appears twice; the `; relay resume` suffix once; `stopped:origin-moved`, `MergeSHA|merge-on-green` and `prompt_hash` absent from the night and run tests (each reads 0); `KillGrace` absent from `night.go` (`run.go` holds it). The fake claude already honours `FAKECLAUDE_IGNORE_TERM`; `FakeLedger.Err` fails every append. `TestPlanEqualsRun` pins the spawn line whole at `night_test.go:295`; the suffix is `night.go:445`; the live line is written at `run.go:138`; the swallowed append is `run.go:86`.

## 5. Forbidden reads

Every other repo; `~/.claude/projects/`; `~/.local/state/agent-relay/`; any ledger outside `t.TempDir()`; the brief; `design/relay-r1.md` beyond the sections named; `design/review-ar2.md` beyond the entries named; `design/ux-review.md` beyond `## Handover`; `.claude/worktrees/`; any file over 8 KB whole unless in `may touch`.

## 6. Report (capped: one table, at most five bullets, under 1,500 words)

The table: each acceptance line, its exit and its last output line on the branch. Bullets: the printed lines before and after (plan's spawn line, the night's red line, the report's stopped section); every choice made inside the design with the relay-r1 sentence it changed; the one open fork for the owner, if any, with its consumer; what was left undone. The last line: the branch name and its head sha.
