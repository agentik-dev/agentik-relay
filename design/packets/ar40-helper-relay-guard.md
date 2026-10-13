# Packet AR40: a helper outside a row runs no relay verb that writes

| item | value |
|---|---|
| from | `design/relay-r1.md` §15 (the guards; `NestedRun`, AR11, AR17B); claude-code-session `design/research.md:29` (every hook's stdin carries an optional `agent_id` / `agent_type`); the observation below |
| pin | agent-relay `main` at 1ac9896 plus this packet's own commit; worktree `.claude/worktrees/ar40-helper-relay-guard`, branch `ar40-helper-relay-guard` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state |
| may touch | `internal/hooks/`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/hooks -run '^TestAR40_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR40_'` reads 3 |
| budget | 60 turns, 80k context, `claude-opus-5-5`; one commit for the act; `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`internal/hooks/guards.go:47-56`: `NestedRun` denies, "in a sitting (RELAY_ROW set …)", a relay that "starts a night, gates, resumes, or plans without --rows", and `go build` / `go install` of `cmd/relay`. `guards.go:792-816`: `guards` adds `NestedRun` only when `inRow` (`RELAY_ROW` or `CLAUDE_SESSION_HELPER` set). `guards.go:993-1003`: `preToolUse` reads `tool_name`, `cwd` and `tool_input`, and not `agent_id` or `agent_type`.

What was seen. On 2026-09-29 at 23:56, a review helper of a plain Claude session (no `RELAY_ROW`) built a relay binary and ran `relay plan` four times; the live night stopped `lock-moved` (fixed at the binary in ar36). On 2026-09-30 a read-only `agent-relay:adversary` helper, whose prompt forbade it, ran `relay run --orchestrate`. It was refused only because a gate held the lock. Both helpers ran outside a row, so no guard applied: the rule lived only in their prompts.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar40-helper-relay-guard` off `main`. The charter is `history/sittings/2026-09-30-7-ar40.md` in doclint's five-part shape, opened at the act, closed before the last commit, no home path. The act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is.

1. **A helper runs no relay verb that writes.** `preToolUse` reads `agent_id` and `agent_type`. When either is non-empty (the call is a subagent's) and the tool is Bash, a new guard `HelperRelay` (`helper-relay`, why: "a helper never builds the relay, starts, gates, resumes, plans, stops, kills or cleans a night; the session that spawned it does") applies whether or not the hook is in a row. Its shape is `nestedRun`'s argv reading, with the verb set widened to `run`, `plan`, `gate`, `resume`, `stop`, `kill`, `clean`, and `plan --rows` denied too. `go build` and `go install` of `cmd/relay` stay denied. `relay status`, `relay report` without `--write`, `relay attach --print`, `relay doctor`, `relay hook`, `make check` and `go test ./cmd/relay …` pass. The main thread (no `agent_id`, no `agent_type`) is unchanged, in a row and out of one. Tests (hooks): `TestAR40_1_HelperDeniedWriteVerbs` feeds a Bash payload with `agent_id` and `agent_type: agent-relay:adversary` and no `RELAY_ROW`, one per verb above plus `relay run --orchestrate` and `go build -o x ./cmd/relay`, and expects `deny` naming `helper-relay`. `TestAR40_1_HelperReadVerbsPass` feeds the passing lines above from a helper, and expects no output. `TestAR40_1_MainThreadUnchanged` feeds `relay run --orchestrate` and `relay stop` with no `agent_id` and no `RELAY_ROW`, and expects no output.

After the act, §15's guard table gains a `helper-relay` row with one sentence: the guard keys on the hook's `agent_id` / `agent_type`, so a helper of any session is held, while the sessions that start nights are not.

## 3. Interfaces (signatures, not prose)

`var HelperRelay = Guard{...}` beside `NestedRun`; a shape func `helperRelay(words []string) bool` that shares `nestedRun`'s binary finding (factor it; do not copy it). `preToolUse`'s input struct gains `AgentID string \`json:"agent_id"\`` and `AgentType string \`json:"agent_type"\``.

## 4. Findings that name this row

`internal/hooks/guards.go:47-110` (`NestedRun`, `relayPath`, `nestedRun`); `:792-816` (`guards`); `:824` (`inRow`); `:993-1036` (`preToolUse`). The existing nested-run tests (search `NestedRun` in `internal/hooks/*_test.go`) show the payload shape.

## 5. Forbidden reads

Other repos but the one research line named in `from`; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: the red test on `main`; what was left. Last line: branch, head sha.
