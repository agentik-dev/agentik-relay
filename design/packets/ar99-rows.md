# Packet AR99: resync recordlint, run it in doclint, the NICE as a table

Copied in from agentik-hq `history/sessions/BRIEF-2026-11-01-d27-ar99-r17.md` (packets d27, AR99, r17, shared by three repos), cut to this repo's row and reshaped to doclint's six sections; no ruling moved. Home paths read as repo names, so hygiene's `home` group finds none.

| item | value |
|---|---|
| from | agentik-hq HANDOFF 18 (build 7, the owner's word of 2026-11-01); agentik-hq `design/05-records-as-tables.md` §2, §3, §4, §7 items 6, 7, 8; hq entry 6 |
| pin | `main` at `0063660`; branch `ar99-rows`, worktree `.claude/worktrees/ar99-rows`; agentik-commons main `eff4fd5` (`scripts/recordlint` `d6c5c217`, on `env python3`) |
| wall | this repo's worktree only; agentik-hq, agentik-commons and the other agentik repos read at main, never written, except that agentik-commons' `scripts/sync <this worktree>` writes into this worktree; no push, `--amend`, rebase, merge, real `make install`, real `install-cli`; `claude` only as `claude plugin validate` and `claude plugin test plugin`; a test never writes under the live `.local`, `.claude`, `.config` of the home; never touch the home's `.config/agentik/hooks-follow-launcher`; no relay verb, no relay binary built or run outside `make`'s test harness; no `kill`, `pkill` or signal from a script; never a gate verb under a temp HOME; zsh here, `bash -c` for bash syntax; `rm -r` only as `"${VAR:?}"` |
| may touch | `scripts/` (by `sync` only), `plugin/bin/` and `plugin/lib/` only as `sync` places its canonical copies, `Makefile` (the doclint target), `NICE_TO_HAVE.md`, `history/NICE-2026-11-01.md` (by `git mv` only), `.hygiene-allow`, `plugin/.claude-plugin/plugin.json` and what a test pins to its version (only if `plugin-version-check` asks), `design/packets/ar99-rows.md`, `history/sittings/*.md`, `history/sessions/NEXT` |
| acceptance | §2's acts; `scripts/recordlint NICE_TO_HAVE.md` exits 0 on the new NICE (main: no `scripts/recordlint`); `make doclint` runs it (main: it does not) |
| verify | `agentik session gate run <absolute worktree path> -- /bin/sh -c "make check > <scratch log> 2>&1"`, then `agentik session gate wait <id>` in one background Bash, last. It exits 0 whatever the job's code: read the code from its line |
| budget | One commit per act. **Stop at 100k context or 70 tool calls:** commit the act in flight, write the exact resume line, report. `claude-opus-5-5`; the charter opens with doclint's five rows |

## 1. What the source says

The last three repos (agentik-decisions d27, this repo AR99, agentik-respeak r17) take design/05's tables, as commons, agents, session and hq did on 2026-11-01. **Not here:** any other repo; any change to doclint; code. Read first, by sections: design/05 §2, §3, §4; commons `GUIDE.md` §9's rows paragraph; `scripts/recordlint`'s head comment; this repo's `Makefile` doclint and hygiene targets.

This repo's row: Go; `scripts/doclint` and `DOCLINT_SHA` untouched; the NICE is 150 KB, grepped for S1 to S3, never read whole; design/05 counts 12 S1 to S3 candidates here.

## 2. Rulings

0. This packet, committed first, as this repo's packet file (its `design/packets/` shape, reshaped so doclint and hygiene pass, as earlier packets were).
1. **Resync.** agentik-commons' `scripts/sync <this worktree>`; commit what it places. If it changes anything beyond `scripts/recordlint` and its `vendored.sha256` line, list each path in the report, and run the tests before committing: a canonical copy that breaks a test of today's behaviour is a stop.
2. **Makefile.** The `doclint` target runs `scripts/recordlint` after its existing lines.
3. **The NICE.** design/05 §3: `git mv NICE_TO_HAVE.md history/NICE-2026-11-01.md`, and in that commit `history/NICE-2026-11-01.md: sibling` under the `NICE_TO_HAVE.md: sibling` row of `.hygiene-allow` (same group and a matching comment). A new `NICE_TO_HAVE.md`: a title, one head paragraph naming the archive, then `| id | level | area | finding | evidence | from |` under a `##`, holding the S1 to S3 items confirmed open on main by a `file:line` or a test; ids from `n1`. S4 and unlevelled stay archived. An empty table is fine when none is open; the head paragraph says so.
4. **The version**, only if `plugin-version-check` asks: the patch.
5. The gate, last.

## 3. Interfaces

No Go code changes. `make doclint` runs the pinned `scripts/doclint`, then `scripts/recordlint` (exit 0 clean, 2 on a finding, 1 on usage). `scripts/vendored.sha256` gains the line `sync` writes for `scripts/recordlint`. `NICE_TO_HAVE.md` becomes a table that recordlint holds; the old file is frozen at `history/NICE-2026-11-01.md`, never edited.

## 4. Findings that name this row

The stop: stop and report, committing what is there, on `sync` placing a copy that breaks a test; hygiene red on a file that cannot be allowed without a new reason row; an S1 or S2 item found open (report it, do not fix it). A stop is a result.

## 5. Forbidden reads

The archive or the 150 KB NICE whole (grep, then line ranges); a relay ledger or a reference snapshot whole; anything under the home's `.local/state` or `.claude/projects`; the private hygiene list; transcripts.

## 6. Report

At most eight lines: the branch and sha; each act's commit; what `sync` placed; the gate's id and its code from its line; the suite's counts; the NICE rows kept (ids, levels, one line each); if stopped, the exact resume line.
