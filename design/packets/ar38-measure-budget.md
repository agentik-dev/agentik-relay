# Packet AR38: the measure's budget, set once and named plainly

| item | value |
|---|---|
| from | `design/relay-r1.md` §15 (the measure's budget) and §3 (the queue's defaults); the observation below |
| pin | agent-relay `main` at `281cc78` plus this packet's own commit; worktree `.claude/worktrees/ar38-measure-budget`, branch `ar38-measure-budget` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process, fake clocks for time. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop` or `kill` against the account's state |
| may touch | `internal/queue/`, `internal/night/`, `internal/relay/`, `cmd/relay/`, `design/relay-r1.md`, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/queue -run '^TestAR38_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR38_'` reads 1; `go test ./internal/night -run '^TestAR38_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR38_'` reads 3 |
| budget | 100 turns, 120k context, `claude-opus-5-5`; one commit per act (1 and 2); `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`bound.go`: "DefaultMeasureSeconds is `--measure-seconds`' default: the whole plan's" budget, 600. `night.go:541-543`: "a baseline the budget left unrun anchors nothing (§15)", then `stopWith(relay.Budget)` with the detail `%d checks unrun: budget (--measure-seconds)`. `types.go:85`: `Budget StopReason = "budget"`, beside `budget-reached` for the usd cap.

What was seen on 2026-09-29. A repo whose check takes about ten minutes under load ran `relay run` with the default. The measure ran out with 8 checks unrun, and the night stopped `stopped:budget` before any row. The owner's session read `budget` as money. Every later run had to pass `--measure-seconds 3000` by hand, since the queue cannot carry it.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar38-measure-budget` off `main`. The charter is `history/sittings/2026-09-30-5-ar38.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is.

1. **The queue carries the measure's budget.** The queue's `defaults` gain `measure_seconds`, an integer from 60 to 14400; anything else is a queue problem that names the key. The order is the flag, then the queue's default, then 600. `relay plan` and `relay run` print the budget used and where it came from (`measure: 3000 s (queue)`), and the `plan` ledger lines carry it. Test: `TestAR38_1_QueueMeasureSeconds` (queue) parses 3000, and refuses 0, 59, 14401 and a string. `TestAR38_1_MeasureSecondsOrder` (night): the flag beats the queue, the queue beats 600, and with neither it is 600.
2. **The night names the measure, not a budget.** The stop reason `budget` becomes `measure-timeout` (`relay.MeasureTimeout`). Its detail reads `<n> checks unrun: the measure ran out of time (<s> s, from <source>); raise measure_seconds in the queue or pass --measure-seconds`. The exit code stays as it was. An old `status.json` or ledger line that says `budget` still prints as it is. A line's own `unrun: budget` status is unchanged. Test: `TestAR38_2_MeasureTimeoutNamed` (night) runs a night whose fake measure outlasts its budget on the fake clock, and expects `stopped:measure-timeout`, that detail, no spawn, and the same exit code as before.

After the acts, §15 and §3 each gain one sentence, and every place that lists stop reasons (the design, the status legend if any, the tests' reason lists) names `measure-timeout`.

## 3. Interfaces (signatures, not prose)

`queue.Row` (the defaults row) gains `MeasureSeconds int` (`yaml:"measure_seconds"`). `relay.MeasureTimeout StopReason = "measure-timeout"` replaces `relay.Budget` for this stop; keep `relay.Budget` only if old records need it for display. `night.Options.measureBudget` takes the queue's value into account, and reports its source.

## 4. Findings that name this row

On `main` at `281cc78`: `internal/night/bound.go:195-210` (`DefaultMeasureSeconds`, `measureBudget`); `internal/night/plan.go:150-162` (`MeasureSeconds`), `:306-317` (the bound shell); `internal/night/night.go:540-543` (the stop); `internal/relay/types.go:85` (`Budget`), `:189` (the reasons list); `internal/queue/queue.go:26` (`Defaults Row`), `:185-205` (applying defaults); `cmd/relay/main.go:413` (`--measure-seconds`).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: per act the red test on `main`; what was left. Last line: branch, head sha.
