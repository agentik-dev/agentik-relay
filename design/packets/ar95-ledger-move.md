# Packet AR95: the relay's portfolio entries come home as entries 13 to 16

| item | value |
|---|---|
| from | agentik-hq `DECISIONS.md` entry 4; portfolio entries 21, 22, 23, 24 and 26 (§1); `DECISIONS.md` entries 1, 2, 4, 7, 10, 12; relay-r1 §5, §12, §14; GUIDE §8, §9; the survey `research/synthesis/tools-relay.md`'s map |
| pin | `main` at 00843a9 plus this packet's own commit; a linked worktree, branch `ar95-ledger-move` |
| wall | AR94's: this repo only; no push, `--amend`, rebase, device, ssh or network; no `relay` binary by hand or outside `make`'s harness; no `relay` or `agentik relay` verb on the account's state; never `claude`. No `make test`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `agentik decisions render`, `make doclint`, `make hygiene`, one at a time at `nice -n 10`, each shell line with `PYTHONDONTWRITEBYTECODE=1`. One `make check`, last, once `ps -axo command \| grep -E '(^\|/)make( \|$)' \| grep -v grep` prints nothing, waited for. Before the report, `agentik decisions render --check` and `make doclint` exit 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `DECISIONS.md`, `CLAUDE.md` (new, by render), `.hygiene-allow` (two lines), `OPEN-QUESTIONS.md` (one line), `NICE_TO_HAVE.md` (two marks, one section), `design/session-layer-r1.md`, `design/relay-orchestrator-r1.md` (a Status line each), `design/packets/ar95-ledger-move.md`, `history/sittings/*.md`, `history/sessions/NEXT`. Nothing else |
| acceptance | `grep -c -E '^1[3-6]\. \*\*' DECISIONS.md` reads 4 (main: 0); `sed -n '/^## In force/,/^## Proposed/p' DECISIONS.md \| grep -c -E '^[0-9]+\. \*\*'` reads 16 (main: 12); `cat CLAUDE.md 2>/dev/null \| grep -c -E '^- [0-9]+ '` reads 16 (main: 0); `grep -i -E 'portfolio entry (2[1-4]\|26)\|portfolio.s relay entries' DECISIONS.md design/*.md \| grep -v -c 'until 2026-10-17'` reads 0 (main: 3) |
| budget | 50 turns, 180k context, `claude-opus-5-5`; three acts, one commit each; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says (quoted)

agentik-hq entry 4: the four "move to agentik-relay's ledger, rewritten without household names". They read as §2's text but for these spans (`[cut]`: a household name, path, stand-in, sitting, or the notifier). 21: "The child never pushes;", "The Go relay in agentik-relay", "The stand-in retired on 2026-09-28: [cut]". 22: the copy-in and packet-writer sentences (entries 1 and 2) and a helpers clause [cut]. 23: its two "A red row is parked" sentences, "the launcher of entry 17". 24: "An [cut] topic is optional". Each provenance names the [cut] entries it left on 2026-09-29. Live citations on main, 3: entry 12's "Portfolio entry 23's amendment gains this sentence."; session-layer-r1's Status line, "(agentik-commons; portfolio entry 26)"; relay-orchestrator-r1's last sentence, "The portfolio's relay entries now live in example-ops's `DECISIONS.md`, …"

## 2. Rulings

Standing rules: the charter, `history/sittings/<date>-<n>-ar95.md`, opens at act 1 and closes before the last commit; no home path; each act's commit message quotes its red line on the pin. No code changes.

1. **Names out** (agentik-hq entry 4): no household repo, product, stand-in, path or sitting; dates kept; provenance as below; this ledger's entries cited, not restated; 23's launcher is `agentik session run`; 22's helpers clause stays behind.
2. **Act 2, the ledger**: the text below under its headings, `### The night` new and first (no section holds the night's shape); 16 in force, cap 25; `updated: 2026-10-17`; entry 12's last sentence becomes "Entry 15 carries this sentence, by citation."
3. **Render**: `agentik decisions render` writes `CLAUDE.md`, new, the block alone; as in agentik-observer, `.hygiene-allow` gains `# AR95: the rendered decisions block names its plugin and old short name.` and `CLAUDE.md: sibling,renamed`.
4. **Hygiene** reads 0; the private list is never printed, copied or read.
5. **Act 3, citations**: live ones take the new numbers; records (what a packet, round or proposal read) keep theirs. session-layer-r1: "portfolio entry 26" becomes "agentik-hq entry 6". relay-orchestrator-r1's last sentence: "The relay entries it names by title live in this repo's `DECISIONS.md` since 2026-10-17, as entries 13 to 16." NICE: the "owed by example-ops" bullets (464, 496) gain `(AR95: entry 15)`; a section "AR95's build (2026-10-17): what it leaves (S4)" maps the old numbers to the new and says relay-r1 §4's "(agent-relay today)" lapsed. OPEN-QUESTIONS, "Decided without asking": one line on the numbers, the section and this split.
6. **Acts**: 1 the copy-in alone; 2 rulings 2 and 3; 3 ruling 5, the charter closed.

### The night

13. **The relay: one child, one worktree, one branch.** The relay runs unattended, headless Claude Code sittings from a program of rows, one child at a time. Each row runs in its own worktree on a branch named for the row, cut from main, or from an earlier row's branch when rows form a chain the owner merges in order. A row id is one word. The child never pushes (entry 4); the driver reads its ledger line, never its transcript. The driver must show a live view: the rows, the acts as commits land, the live line. The Go relay of this repo, run as `agentik relay`, is the only driver. (2026-09-22; portfolio entry 21 until 2026-10-17, moved under agentik-hq entry 4; amended 2026-10-13, the owner's word at 14:15, agentik-r1)

16. **How a row and a night stop.** A classifier refusal is final for the run. When the auto-mode classifier refuses an act, such as a merge, the orchestrator never retries or routes around it. The act becomes the owner's, and dependent rows chain from the refused row's branch as their base. The ceiling kill is for runaway tasks. A relay child is asked to stop at 90% of its row's context ceiling, so it lands the act in flight. The hard kill comes at 120% of the ceiling or later. Each exit records what would tune the kill; it is tuned from those records, never made more rigid. A night's stops and questions go to its log, and the owner reads them in the morning. A push topic is optional, and the owner is not notified overnight (entry 7). (2026-09-23, 2026-09-25 and 2026-09-28, three entries merged 2026-09-29; portfolio entry 24 until 2026-10-17, moved under agentik-hq entry 4; amended 2026-09-29, the owner's word in the selector, given with entry 7)

### Packets

14. **A packet is a slice that fits one sitting.** A packet is a small, finishable slice with an acceptance table; about four acts fit one builder. Each target repo is the home of its own queue and its own packets, as the Go relay is designed. The builder's copy-in at its first act, the skipped copy-in a row may declare, and the one packet-writer sitting left, for a packet that needs the target repo's toolchain first, are entries 2 and 1. (2026-09-24; portfolio entry 22 until 2026-10-17, its relay share moved under agentik-hq entry 4, its helpers clause kept there)

### The gate

15. **The gate, and what a red row does.** A child whose result is an error is red. Otherwise the row is green only when three checks hold: the child made a commit, its tree is clean, and the verify line is green or exits as its baseline on main did. The verify line is the repo's own `make check` by default, cut at entry 10's limit. A line already red on main is compared, never failed on its own. A gate that could not run is unrun, and unrun is red: unknown is never a pass. A red row is parked with its reason, never retried the same day; a new attempt is a new row with a new id. A gate red moves the night on; a sitting failure stops it. An expired login stops the whole run, not one row: the child's uncommitted files stay in its tree, and the next attempt is free. A usage limit pauses the run and no longer stops it: the turn it ended stands as it is, the child's uncommitted files stay in its tree, a wake armed across the plan's reset re-enters the loop, and the sitting reads its charter's act in flight before it starts anything, so no act is duplicated. One model's own limit pauses only that model's rows (entry 12). agentik-session's launcher, `agentik session run`, restarts a sitting the same way. (2026-09-22 and 2026-09-23, two entries merged 2026-09-29; portfolio entry 23 until 2026-10-17, moved under agentik-hq entry 4; amended 2026-10-11, the owner's word of 2026-10-10 21:06: a usage limit pauses the run; amended 2026-10-12 by entry 12)

## 3. Interfaces (signatures, not prose)

`N. **Title.** body (date; provenance)`, one line each, under `### <area>` in `## In force`; touched: The night (new), Packets, The gate, Usage limits; `CLAUDE.md`'s block. No code.

## 4. Findings that name this row

agentik-hq `HANDOFF.md` item 11. Nothing else.

## 5. Forbidden reads

The portfolio ledger (§1 quotes it); transcripts; the relay state; the private hygiene list.

## 6. Report (capped)

At most 120 words: lines 1 to 4; the wall's last three checks and `make check`; each act's red line; choices beyond §2. Last: branch, head sha. Fork: none.

