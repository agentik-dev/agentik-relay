# Packet AR101: the relay names its children's effort

| item | value |
|---|---|
| from | the owner's answers of 2026-11-11 to agentik-hq's prompting-guide audit (`history/sessions/AUDIT-2026-11-11-prompting-guides.md`, findings-C finding 1, findings-B finding 2), quoted in §1; agentik-hq's brief `BRIEF-2026-11-11-relay-effort.md` |
| pin | `main` at `0252c72` plus this packet's own commit; branch `ar101-effort`, worktree `.claude/worktrees/ar101-effort`; nothing linked from main (a Go repo, no venv) |
| wall | this repo's worktree only; no push, `--amend`, rebase, merge, real `make install`, plugin update; never build a relay binary by hand, never run one outside `make`'s test harness, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; never `claude`; `make check` only through the gate queue (`agentik session gate run`), never a bare `make`; no `DECISIONS.md` or rendered block edited |
| may touch | `internal/queue/queue.go`, `internal/queue/ar101_test.go` (new), `internal/night/plan.go` (`SpawnArgv`), `internal/night/plan_test.go` (`TestPlanOverFakes`'s spawn line), `internal/night/ar101_test.go` (new), `internal/night/ar101_run_test.go` (new), `plugin/agents/builder.md`, `plugin/agents/adversary.md`, `plugin/agents/attendant.md` (one frontmatter line each), `plugin/.claude-plugin/plugin.json` (the version), `internal/packet/agentik_r1_test.go` (the version), `internal/packet/ar101_test.go` (new), `design/relay-r1.md` (§3, §7), `design/packets/ar101-effort.md`, `NICE_TO_HAVE.md` |
| acceptance | `go test ./internal/queue -run '^TestAR101_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR101_'` reads `1` (main: `0`); `go test ./internal/night -run '^TestAR101_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR101_'` reads `6` (main: `0`); `go test ./internal/packet -run '^TestAR101_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR101_'` reads `1` (main: `0`); `grep -c -x 'effort: high' plugin/agents/builder.md` reads `1` (main: `0`); `grep -c -x 'effort: high' plugin/agents/adversary.md` reads `1` (main: `0`); `grep -c -x 'effort: low' plugin/agents/attendant.md` reads `1` (main: `0`); `git diff main -- plugin/agents \| grep -c -E '^[-+][^-+]'` reads `3` (main: `0`); `grep -c -F '"version": "0.2.1"' plugin/.claude-plugin/plugin.json` reads `1` (main: `0`); `grep -c -F -- '--effort <e>' design/relay-r1.md` reads `1` (main: `0`) |
| budget | 60 turns, 180k context, `claude-opus-5-5` at `high`; four acts, one commit each: 1 the tests, red; 2 the key and the spawn line; 3 the agent files and the version; 4 the docs |
| verify | `make check` |

## 1. What the source says (quoted)

The owner, 2026-11-11, on the finding that every builder, refuter and relay child ran at `xhigh` effort by inheritance, never measured: "ok set the defaults for these. Opus 5.5 can be high starting point." To "Drop effort to `high` now and measure `xhigh`?" the owner answered "yes".

What the audit found at the pin: `SpawnArgv` passes `--model` and no `--effort`; the queue has no such key; the relay strips every `CLAUDE*` variable from a child's environment, so the child's effort came from the Mac's settings file, which said `xhigh`. The plugin's three agent files pin a model and no effort, so a helper started from one takes its caller's level. A queue key is what lets a later trial run some rows at `medium`.

## 2. Rulings

1. **The queue key.** `effort` is an optional key in `defaults` and in a row (not inside `budget`). Its values are `low`, `medium`, `high`, `xhigh` and `max`, the `claude --effort` levels; any other value is a row problem in `Validate`, as the other key problems are, so the plan is invalid and `run` spawns nothing. A row inherits the defaults' value as it inherits every other key. Unset (or empty) is no value. agentik-session's unattended loop gains the same key, the same name and values, in its own packet.
2. **The spawn line.** `SpawnArgv` passes `--effort <level>` right after `--model <m>`: the row's, else the defaults', else `high` (`queue.DefaultEffort`). A child never inherits the interactive default. The plan's printed spawn line, `run --dry`'s and the ledger's `spawn` line (`argv`) carry it, since each is that argv; no new ledger key. An orchestrator row's sittings take the same rule through the same function. A continuation keeps it (`continueArgv` rewrites only the session, turns and money).
3. **The agent files.** `effort: high` after `model:` in `builder.md` and `adversary.md`, `effort: low` after `model:` in `attendant.md`; nothing else in them changes.
4. **The docs and the version.** `design/relay-r1.md` §3 (the example and a paragraph) and §7 (the spawn line) name the key and the flag. The plugin goes `0.2.0` to `0.2.1` (a patch: only agent frontmatter changed), with the three `0.2.0` of `internal/packet/agentik_r1_test.go`. agentik-commons' queue-format line is not this repo's; the hand-back names it.
5. **No ledger entry** is added or changed.

## 3. Interfaces (signatures, not prose)

`queue.Row.Effort string`; `queue.Efforts = []string{"low", "medium", "high", "xhigh", "max"}`; `queue.DefaultEffort = "high"`; `func (r queue.Row) EffortLevel() string`; `night.SpawnArgv` keeps its signature. Tests: `TestAR101_1_EffortKey` (queue); `TestAR101_2_NoEffortSpawnsHigh`, `TestAR101_3_DefaultsThenRow`, `TestAR101_4_BadValueSpawnsNothing`, `TestAR101_5_DryRunShowsLevel`, `TestAR101_6_OrchestratorCarries`, `TestAR101_7_LedgerSpawnLine` (night); `TestAR101_8_AgentEffort` (packet).

## 4. Findings that name this row

None. Not here: the gate, the stop rules, the prompt's fixed text and the continuation paths; the helper-line allowlist of `internal/hooks/guards.go` (a row's own `claude -p` helper still may not pass `--effort`); the continue-once change to entry 15, which is a later packet.

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or night record; transcripts; `history/NICE-2026-11-01.md` whole.

## 6. Report

The branch and head sha; the gate job and its result line; the acceptance table at the pin and now; the choices; what is left.
