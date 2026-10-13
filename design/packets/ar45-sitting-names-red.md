# Packet AR45: a sitting line names its red checks

| item | value |
|---|---|
| from | `design/relay-r1.md` §15 (the ledger's `sitting` line); `NICE_TO_HAVE.md`, the observer's check-in 11 (2026-09-30 22:00) |
| pin | agent-relay `main` at 67fc739 plus this packet's own commit; worktree `.claude/worktrees/ar45-sitting-names-red`, branch `ar45-sitting-names-red` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state |
| may touch | `internal/night/run.go`, `internal/night/*_test.go`, `internal/gate/gate.go`, `internal/gate/*_test.go`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^TestAR45_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR45_"` reads 2 |
| budget | 60 turns, 120k context, `claude-opus-5-5`; one commit per act; `make check` in the foreground, then the last commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`internal/night/run.go`, the comment above `sitting.write`: the line carries "gate as an object holding result and verify". `write` builds it as `{"result": …, "verify": …}` and adds `clean`, `commit` and `is_error` when the gate gathered. `internal/gate/gate.go` marks the checks one by one as `gate` events (`branch`, `exit`, `commit`, `clean`, `origin`, `touch`, and for an orchestrator `charter`, `queue` and `merges`, then `packet-source`, the acceptance lines and verify).

What was seen on 2026-09-30, night 2026-09-30-8, row g2b, in the account's ledger: the `sitting` line reads `"gate":{"clean":true,"commit":true,"is_error":false,"result":"red","verify":{…,"exit":0,"status":"green"}}` and `"reason":""`. Every check the line shows is green and the result is red. The red check was `touch`, which only the separate `{"check":"touch","event":"gate","status":"red"}` line carries. A reader of the sitting line, `relay report` among them, cannot say why the row is red.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar45-sitting-names-red` off `main`. The charter is `history/sittings/<the build date>-<n>-ar45.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is. No existing key of the `sitting` line changes its name, type or meaning.

1. **The sitting line's `gate` object gains `red`** (act 1): the names of the checks that read red, as a list of strings in the order the gate marks them, each name the `check` value its own `gate` event carries. The list is empty (`[]`, never absent and never null) when the result is green, and when the gate did not gather. Test `TestAR45_1_SittingNamesRed`: a row whose branch changes one path outside its `may touch`, all else green, writes a sitting line with `result` `red` and `red` equal to `["touch"]`; a green row writes `[]`; a row red on verify alone writes the one name verify's `gate` event carries.

2. **One source for both** (act 2). The `gate` events and the sitting line's `red` are read from the same facts through one function, so the two cannot disagree. Test `TestAR45_2_OneSource`: for a row red on two checks at once (`touch` and `clean`), the sitting line's `red` equals, in order, the `check` names of that row's `gate` events whose status is red in the same ledger.

3. **The fix round** (review round 1: 0 blocking, three reds still unnamed). `red` names every check the gate does not pass, an `unrun` acceptance line and an unrun verify among them, each by its `gate` event's `check` value; then `main-moved` when main moved before the gate, and `orphans` when orphans were swept. These last two have no `gate` event behind them. Ruling 2's test holds for the names that have one.

After the acts, the design's `sitting` line (§5) names the new key in one sentence.

## 3. Interfaces (signatures, not prose)

No exported signature is removed or changed. A new exported function or method in `internal/gate` that returns the red check names is yours to shape and name.

## 4. Findings that name this row

`internal/night/run.go:640-672` (`sitting.write` and its comment); `internal/gate/gate.go:1018-1050` (`mark` and the checks in order); `internal/night/run_test.go:335` (a test that reads the line's `gate` object).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: each act's red test on `main`; the function you added and who calls it; what was left. Last line: branch, head sha.
