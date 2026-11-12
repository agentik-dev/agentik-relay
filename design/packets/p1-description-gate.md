# Packet P1: the plugin's descriptions under 1,024 bytes, held by `make check`

| item | value |
|---|---|
| from | agentik-hq's brief `BRIEF-2026-11-11-plugins-p1-description-gate.md`; hq design/09 §1 (F7), §2 (the row "A plugin's descriptions"), §6, §9 step 4; the owner's yes to question 7 of hq design/09, quoted in §1; agentik-commons packet p1 (`scripts/plugin-description-bytes`, its branch `p1-description-gate` at 06a2cf5) |
| pin | `main` at `a3c501d` plus this packet's own commit; branch `p1-description-gate`, worktree `.claude/worktrees/p1-description-gate`; nothing linked from main (a Go repo, no venv) |
| wall | this repo's worktree only; no push, `--amend`, rebase, merge, real `make install`, plugin update; never build a relay binary by hand, never run one outside `make`'s test harness, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; never `claude`; `make check` only through the gate queue (`agentik session gate run`), never a bare `make`; no `DECISIONS.md` or rendered block edited |
| may touch | `plugin/commands/*.md` (one frontmatter line each), `plugin/.claude-plugin/plugin.json` (the version), `internal/packet/agentik_r1_test.go` (the version), `internal/packet/p1_test.go` (new), `scripts/plugin-description-bytes` (new, vendored), `scripts/vendored.sha256`, `Makefile`, `design/packets/p1-description-gate.md`, `NICE_TO_HAVE.md` |
| acceptance | `go test ./internal/packet -run '^TestP1_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestP1_'` reads `3` (main: `0`); `scripts/plugin-description-bytes plugin \| tail -n 1` reads `total 578 of 1024 B` (main: `total 1204 of 1024 B`, exit 1); `grep -l -x 'disable-model-invocation: true' plugin/commands/*.md \| wc -l` reads `6` (main: `0`); `git diff main -- plugin/commands \| grep -c -E '^[-+][^-+]'` reads `6` (main: `0`); `grep -c -F '"version": "0.2.2"' plugin/.claude-plugin/plugin.json` reads `1` (main: `0`) |
| budget | `claude-opus-5-5` at `high`; three acts, one commit each: 1 the tests, red; 2 the six commands hidden and the version; 3 the vendored script in `make check` |
| verify | `make check` |

## 1. What the source says (quoted)

The owner, 2026-11-11, his yes to question 7 of hq design/09: "The plugins hide the commands only the owner types, under 1,024 B a plugin: yes." And: "A focus should be on avoiding Claude.md bloat".

hq design/09 §1, F7: a skill's or command's description is in context in every main session unless it sets `disable-model-invocation: true`; a general-purpose helper carries the skill list and the agent-type list too. §9 step 4: "`disable-model-invocation: true` on the commands only the owner types; the 1,024 B gate in `make check`, landing in each repo in the same change that brings its descriptions under it, since agents, relay and decisions are over it today".

At the pin, by commons' script: 1,204 B, over by 180 (doctor 92, gate 106, plan 90, report 104, resume 122, status 112; adversary 201, attendant 189, builder 188).

## 2. Rulings

1. **R3, hidden.** All six commands set `disable-model-invocation: true`, on a line after `description:`. Nothing tells the model to start one by name: the three agent files name the binary's verbs (`agentik-relay status --json`, `agentik relay resume <row>`), never a `/relay:` command; the hook text is the binary's and names none (`grep -rn 'relay:(doctor|gate|plan|report|resume|status)'` outside `design/`, `history/`, `docs/` finds only `README.md`, its source, `plugin.json`'s description and a test). Typed, each runs as before; its body does not change.
2. **R4.** The three agents' 578 B are under the ceiling: no description is reworded.
3. **R5.** Each agent file already lists its `tools`, none with Skill or Agent.
4. **R1 in the check.** `scripts/plugin-description-bytes`, copied byte for byte from commons' branch, its sum added to `scripts/vendored.sha256` where `sync` writes it (after `plugin-version-check`); `check-unlocked` gains `descriptions`, `$(PY) -B scripts/plugin-description-bytes plugin`.
5. **R6.** The plugin goes `0.2.1` to `0.2.2` (a patch: only command frontmatter changed), with the three `0.2.1` of `internal/packet/agentik_r1_test.go`. `plugin.json`'s description and the README name `/relay:resume` for the owner to type; they quote no command's description, so neither changes.

## 3. Interfaces (signatures, not prose)

`scripts/plugin-description-bytes [--max BYTES] [PLUGIN_DIR]`: commons' (packet p1 there), exit 0 at or under 1,024 B. Tests: `TestP1_1_CommandsHidden` (each of the six sets it, its body's sha256 the pin's), `TestP1_2_AgentsDescribed` (each agent's description non-empty, its tools without Skill or Agent), `TestP1_3_UnderTheCeiling` (the vendored script listed and exiting 0 on `plugin/`).

## 4. Findings that name this row

None. Not here: the agent files' other lines (hq design/09 §9 step 3), another packet.

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or night record; transcripts.

## 6. Report

The branch and head sha; the gate's id and its line; the bytes before and after; the six commands hidden; choices; leftovers.
