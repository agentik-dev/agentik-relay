# Packet AR7: the thirteen lows, ar4's gaps and flake, the live line, gofmt, the rehearsal

| item | value |
|---|---|
| from | `design/relay-r1.md` §1, §5, §6, §9, §11; `design/review-ar2.md` AR2-14, AR2-17, AR2-23 only; the handovers of `history/sittings/2026-09-22-16.md`, `-17.md`, `-19.md`. The builder reads THIS packet and those; never the brief or a ledger |
| pin | agent-relay `main` at f2a763b (cut off `main`; no `base`); worktree `.claude/worktrees/ar7-fixes` |
| wall | this repo only. No push, `--amend`, rebase, device or network; `relay` runs only as `go run ./cmd/relay` under a fresh temp `XDG_STATE_HOME`, `run` only with `--dry` (the real spawn line never runs); `make readme` never; `design/relay-r1.md`, `design/review-ar2.md`, `relay/queue.yaml`, `scripts/*` untouched |
| may touch | `Makefile`, `cmd/relay/*.go`, `internal/effects/*.go`, `internal/gate/*.go`, `internal/night/*.go`, `internal/queue/*.go`, `internal/queue/testdata/*` (new), `internal/relay/gate.go`, `internal/relay/gate_test.go`, `internal/stream/*.go`, `internal/stream/testdata/*` (new); `*` is one path segment (§6's match). Nothing else; the charter is admitted by §6 |
| acceptance | `make check; echo $?` reads 0; `gofmt -l cmd internal \| wc -l` reads 0; `grep -c 'gofmt -l' Makefile` reads 1; `go test ./... -run 'TestAR7_' -v 2>&1 \| grep -c '^--- PASS'` reads 14; `grep -Ec 'pgrep\|pkill' internal/effects/ar2_test.go` reads 0; `grep -c 'o.Plan = p' internal/night/night_test.go` reads 0; `grep -c 'lines, _ := effects.NewLedger' cmd/relay/main.go` reads 0; `grep -c 'sc.Err()' internal/effects/ledger.go` reads 1; `grep -c 'untracked-files=all' internal/effects/git.go` reads 1; `grep -c 'IsLocal' internal/queue/queue.go` reads 1; `t=$(mktemp -d) && XDG_STATE_HOME="$t" go run ./cmd/relay run --dry --rows ar8-resync > "$t/o"; echo "$? $(grep -c 'caffeinate -i claude -p' "$t/o")"` reads `0 1`; `git status --short \| wc -l` reads 0 after the last commit |
| budget | 120 turns, 130k context, `claude-opus-5-5`; 120 because eighteen acts reproduce before they fix; `make check` carries `-race`; one commit per act; checkpoint at 80%; the charter is the first free `history/sittings/2026-09-23-<n>.md` on the branch |

## 1. What the source says (quoted)

§6: "a gate whose facts could not be gathered is `unrun`, never a pass"; the verify line is "green, or same-as-main, never failed on its own". §5: "`gate` an object holding `result` and `verify`", "`stop` (reason, detail)". §11: "the spawn lines `plan` prints equal the `spawn` argv `run` records". AR2-14, AR2-17, AR2-23: thirteen lows, each "not reproduced by the coordinator". ar4's handover: "`relay.Gate.Verify` is never filled by `gather`"; "an object value is left out of the body"; `TestAR2_6` "greps `pgrep -f` machine-wide". ar5's: the live line "shows that message's first event's tool". ar6's: "`gofmt -l` lists `internal/night/night_test.go`".

## 2. Rulings

The owner's standing word: decide inside the design, record each choice in the charter, escalate at most ONE real fork. Every low is reproduced by a `TestAR7_` test first; one that does not reproduce keeps its test as the pin and the charter says so. One act per line:

1. AR2-14 `schedule.go:41`: `PickIn` returns `q.Order()`'s error as a third value; `night.go` stops the night `no-progress` with the text as the `stop` line's `detail` (§1: nothing can start), never `blocked`.
2. AR2-14 `stage.go:48`: `LiveClaude` reads stderr; exit 1 with both streams empty is no claude, exit 1 with stderr text is `lsof: <text>`.
3. AR2-14 `gate.go:172`: `Run` returns the gather's error (`f.err`, or the lost append) fourth; `Hand` passes it on, the night's closure drops it (the `gate` line has `detail`); `gate <row>` prints `unrun: <cause>`, `--json` fills `error`; no new key.
4. AR2-14 `gate.go:61`: `relay.Gate` gains `Skipped []string` (`skipped`); `printGate` prints `exit skipped`, `origin skipped`; `Decide` unchanged.
5. AR2-17 `ledger.go:66`: `Lines` returns `sc.Err()` naming the file.
6. AR2-17 `ledger.go:91`: `capAny` caps through JSON (marshal, unmarshal into `any`, cap every string), named strings and struct fields included.
7. AR2-17 `git.go:60`: `Status` runs `--porcelain --untracked-files=all` and skips only `.claude/worktrees/` paths; other `.claude/` content is dirt.
8. AR2-17 `effects.go:25`: `Last` is stdout's last non-empty line, stderr's only when stdout has none.
9. AR2-17 `queue.go:340`: an id matches `^[A-Za-z0-9][A-Za-z0-9._-]*$` (never `.` or `..`); ids unique case-folded (this Mac's filesystem).
10. AR2-17 `queue.go:87`: a `packet` path that is absolute or not `filepath.IsLocal` is the problem `packet path leaves the repo`, never read.
11. AR2-23 `main.go:984`, `:936`: `report` and `resume` print `relay <verb>: the ledger: <err>` to stderr and exit 2.
12. AR2-23 `TestPlanEqualsRun`: hands `Run` no plan (`o.Plan = nil`) under a deterministic `PlanOptions.UUID`, so the printed plan and the night's own must agree.
13. ar4's gap: `Run` takes the verify line and its plan baseline (`Plan.Verify`), runs it in the worktree after the acceptance lines, fills `g.Verify` (green on exit 0, same-as-main when exit and last equal the baseline's, else red; an absent line is `skipped`) and writes a `gate` line `check: verify`; `Hand` passes no baseline (green only). `gate.verify` is null only when no gate ran.
14. ar4's gap: `noticeValue` renders a map by its `result` string, so a sitting notice reads `gate=green`.
15. ar4's flake: `TestAR2_6` has the line write `$!` to `pid` in the temp dir and polls `syscall.Kill(pid, 0)` until `ESRCH`, killing it on failure; no `pgrep`, no `pkill`.
16. ar5's line: `RunRow` holds the current message's line and writes it when the next message begins (`Fresh`) or at the result: a message streamed as several events shows its own last tool, one line per message.
17. `night_test.go` formatted; the Makefile gains `fmt` (`gofmt -l cmd internal`, exit 2 naming the files) between `vet` and `doclint` in `check`, conventions §5's order kept.
18. The rehearsal (§9, §11): `run --dry --rows ar8-resync` from a temp state directory prints ar8's spawn line and exits 0; nothing staged, nothing spawned. A failure is fixed in `plan` or `run`, never in the queue.

## 3. Interfaces (signatures, not prose)

- `night.PickIn(...) (next []string, blocked bool, err error)`; three test callers follow.
- `gate.Run(fx, st, p, baseline, exit, verify gate.Verify) (relay.Result, relay.StopReason, relay.Gate, error)` with `type Verify struct{ Name string; Baseline *relay.Check }` (the name is `night.VerifyLine`'s input); `Hand` unchanged.
- `relay.Gate.Skipped []string \`json:"skipped"\``; `relay.Check` unchanged.
- `queue.Validate`: problems `id %q: not [A-Za-z0-9][A-Za-z0-9._-]*` and `packet path leaves the repo`; `duplicate id` also case-folded.
- Makefile: `fmt:` target; `check: test vet fmt doclint hygiene validate`.
- Tests, fourteen: `TestAR7_PickInOrderError`, `TestAR7_LiveClaudeLsofError` (a fake `lsof` first on PATH), `TestAR7_HandGateUnrunNamesCause`, `TestAR7_GateSkippedChecks`, `TestAR7_LinesOverflowError` (1 MiB + 1), `TestAR7_CapNamedAndStruct`, `TestAR7_StatusSeesClaudeDir`, `TestAR7_LastIsStdout`, `TestAR7_QueueIDShape` (`..`; `A` beside `a`), `TestAR7_PacketPathInRepo` (`../x.md`), `TestAR7_ReportAndResumeLedgerError` (a directory named `<date>.jsonl` in the ledger dir), `TestAR7_GateFillsVerify`, `TestAR7_NoticeGateObject`, `TestAR7_LiveLineNamesOwnTool` (a new fixture: one message as two events, the `tool_use` in the second).

## 4. Findings that name this row

Baseline on `main` at f2a763b, 2026-09-23: `make check` 0; `gofmt -l` 1 (`night_test.go`, one struct field); Makefile `gofmt -l` 0; `TestAR7_` 0; `pgrep|pkill` 2; `o.Plan = p` 1; `lines, _ :=` 2; `sc.Err()` 0; `untracked-files=all` 0; `IsLocal` 0; the rehearsal line reads `2 0` before the queue rows and `0 1` with them (charter 2026-09-23-1). The code: `schedule.go:41-44`; `stage.go:45-52` (`.Output()`, stderr unread); `gate.go:168-215` (`f.err` reaches only the ledger's `detail`), `:57-60` (skipped exit reads `true`), `printGate` at `main.go:551`; `ledger.go:66-72`, `:88-118`; `git.go:53-65`; `shell.go:23` (one buffer); `queue.go:340-346` (`seen` by exact id), `:87-90`; `main.go:995`, `:1051`; `night_test.go:288`; `run.go:148` (`t.Fresh`), `:269-276` (`"verify": nil`); `notify.go:96-106` (no map case); `ar2_test.go:11-27`.

## 5. Forbidden reads

Every other repo; `~/.claude/projects/`; `~/.local/state/agent-relay/`; any ledger outside `t.TempDir()`; the brief; the relay ledgers under example-mac's `history/relay/`; `design/relay-r1.md` beyond the five sections; `design/review-ar2.md` beyond the three entries; `.claude/worktrees/`; any file over 8 KB whole unless in `may touch`.

## 6. Report (capped: one table, at most five bullets, under 1,500 words)

The table: each acceptance line, its exit and last line on the branch. Bullets: each choice inside the design (each low's fix or non-reproduction, the verify fill, the live line, the rehearsal's output); what was left undone and where it goes; the one open fork, if any, with its consumer. Last line: the branch and its head sha.
