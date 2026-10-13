# Packet AR55: the report's JSON says "not stopped" for a night with no stop line

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`, the items at "`relay report --json` on a running night returns" and "`relay report --json` gives the night's `state` as `""`" (§1) |
| pin | agent-relay `main` at the commit that lands this packet; worktree `.claude/worktrees/ar55-report-state-not-stopped`, branch `ar55-report-state-not-stopped` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state: a real night may be live. No `make check` and no `go test ./...`: the acceptance lines, `go vet ./internal/night ./cmd/relay`, `make doclint` and `make hygiene` only, one at a time. **Python:** every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/report.go` (`BuildReport`, `Markdown`'s title line), `internal/night/ar55_test.go` (new), `design/relay-r1.md` (the `report` verb's row), `plugin/agents/attendant.md` (one sentence), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^TestAR55_" -count=1 -timeout 300s -v 2>&1 \| grep -c "^--- PASS: TestAR55_"` reads 2; `go test ./internal/night -run "^TestAR51_" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./cmd/relay -run "^TestJSONShapesFrozen$" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 30 turns, 120k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

`NICE_TO_HAVE.md` holds this twice. The first is "`relay report --json` on a running night returns `"state":""` while `relay status` reads `running` (night 2026-09-30-4, 12:40)". The second is "`relay report --json` gives the night's `state` as `""` for a live night, where status gives `running` and the text report says "not stopped"". On 2026-10-03 at 15:12 a third read of night 2026-10-03-1 took the empty state for a fault. The observer checked it: `report --json` read `state= ''` while `status --json` read `running`.

The cause is `BuildReport` (`internal/night/report.go:79`). It sets `Report.State` only from a `stop` line (`:119-123`). The markdown fills the gap at render time with `or(r.State, "not stopped")` (`:251`). The JSON has no such fill.

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar55.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. `DECISIONS.md` stays as it is. Each act's test is red on `main`; put the red line in the commit message.

1. **One word for both (act 1).** `BuildReport` sets `State` to `"not stopped"` when the lines hold no `stop` line for the night. Add the constant `NotStopped = "not stopped"` in `internal/night/report.go`. `Markdown`'s title reads `r.State` as it stands, with the same words as today. The report reads the ledger only. It does not say `running`, because a night with no `stop` line may be live or may have died without writing one. `internal/night/ar55_test.go`: `TestAR55_1` builds a report from a `night-open` line and one `sitting` line and reads `State == "not stopped"`, `"state":"not stopped"` in `json.Marshal`, and the markdown title `# Night <n>: not stopped`. `TestAR55_2` adds a `stop` line with `"state": "stopped:count-reached"` and reads that state in all three places.
2. **The records (act 2).** `design/relay-r1.md`'s `report` row gains "`--json`'s `state` is the `stop` line's, or `not stopped` when the night has none". `plugin/agents/attendant.md` gains one sentence: a report state of `not stopped` means no `stop` line yet; read `status` for live or dead. Both `NICE_TO_HAVE.md` items leave with "(landed as AR55)".
3. **What this packet does not do.** It does not read the pid file or `status.json` from the report, rename a JSON key, or change `status`.

## 3. Interfaces (signatures, not prose)

New: `night.NotStopped` (a string constant). Unchanged: `Report`'s fields and JSON keys, `BuildReport`, `Markdown`, `WriteReport`.

## 4. Findings that name this row

`internal/night/report.go:15-31` (`Report`), `:79` (`BuildReport`), `:119-123` (the `stop` case), `:251` (`Markdown`'s title); `design/relay-r1.md:212` (the `report` row); `plugin/agents/attendant.md:16`.

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line with its count, `go vet`'s exit, `make doclint`'s exit and `make hygiene`'s last line. Bullets: each act's red line on `main`; what was left. Last line: branch, head sha.
