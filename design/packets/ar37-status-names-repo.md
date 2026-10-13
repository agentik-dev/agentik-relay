# Packet AR37: a night names its repo

| item | value |
|---|---|
| from | `design/relay-r1.md` §9 (`status`, `stop` / `kill`) and §8 (one relay per account); the observation below |
| pin | agent-relay `main` at the merge of ar36 plus this packet's own commit; worktree `.claude/worktrees/ar37-status-names-repo`, branch `ar37-status-names-repo` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop` or `kill` against the account's state |
| may touch | `internal/night/`, `cmd/relay/`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR37_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR37_'` reads 2; `go test ./cmd/relay -run '^TestAR37_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR37_'` reads 1 |
| budget | 80 turns, 100k context, `claude-opus-5-5`; one commit per act (1 and 2); `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

§8: one relay per account, so the account's state holds every repo's nights in one `nights/` list. `status.go:21-33`: `Status` carries `night`, `state`, `reason`, `rows`, `burn`, `next`, `resume`, `pid`, `started`, and no repo. The ledger's `night-open` line has `root`.

What was seen on 2026-09-29. Another repo's session read `relay status` and saw nights running all evening. The header was `night 2026-09-29-11: ; 0 rows`, which names no repo. The session took the nights for a third repo's, and held two cross-repo files until "that repo's" night ended. `relay stop` prints `relay stop: night <id>: "stop" written`, which does not name the repo either, so an owner cannot tell whose night they are stopping.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar37-status-names-repo` off `main`. The charter is `history/sittings/2026-09-30-4-ar37.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is.

1. **status.json names the repo.** `Status` gains `Root string` (`json:"root"`), the plan's root, written with every status. The dashboard's first line becomes `night <id> in <root>: <state>; …`. The root is printed with the home directory shortened to `~`, read from `HOME`. An old `status.json` with no root prints as today. Tests: `TestAR37_1_StatusCarriesRoot` (night) runs a one-row night and reads `root` in status.json and in the header. `TestAR37_1_OldStatusPrintsAsBefore` (night) prints a status.json with no root, and the header is unchanged.
2. **stop and kill name the repo.** `relay stop` and `relay kill` print `relay <verb>: night <id> in <root>: …` when the night's status names a root. When the cwd is inside a git repo whose top level differs from that root, they add one line: `note: this night runs in <root>, not in this repo`. They still write the file; the account holds one relay. Test: `TestAR37_2_StopNamesRepo` (cmd/relay) covers a night whose status names root A, stopped from a cwd in repo B. It expects both lines, and the control file written.

After the acts, §9's `status` row and the `stop` / `kill` row each gain one sentence. `status --json`'s frozen key list gains `root`.

## 3. Interfaces (signatures, not prose)

`night.Status` gains `Root string`. `NightOptions` or the plan already carry the root (`p.Root`), so no new option is needed. `controlVerb` reads the night's status for its root.

## 4. Findings that name this row

`internal/night/status.go:21-33` (`Status`) and `:166-` (`PrintStatus`); the night's status writer in `internal/night/night.go` (search `status()`), and `p.Root`; `cmd/relay/main.go:1400-1410` (`controlVerb`'s message); the frozen JSON key list test for `status --json` (search `pid` and `started` added by AR33).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: per act the red test on `main`; what was left. Last line: branch, head sha.
