# Packet AR100: entry 13 names the second driver for the trial

| item | value |
|---|---|
| from | agentik-hq entry 14 (the owner's answers to hq questions 14 to 16, 2026-11-08); agentik-hq `design/07-unattended-loop.md` §6, §7 (the sentence), §11; agentik-hq HANDOFF 22; this repo's entry 13 (quoted in §1) |
| pin | `main` at `dc5429e` plus this packet's own commit; branch `ar100-entry13`, worktree `.claude/worktrees/ar100-entry13`; the first row the unattended loop runs here (hq entry 14) |
| wall | this repo's worktree only; agentik-hq read at main, never written; no push, `--amend`, rebase, merge, real `make install`, real `install-cli`; never build a relay binary by hand, never run one outside `make`'s test harness, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`; never `claude`; no `make check` (the land step runs it), no `make test`, no `go test ./...`; `agentik decisions render` and `agentik decisions show 13` are the only tools run; no `kill`, `pkill` or signal; zsh here, `bash -c` for bash syntax; `rm -r` only as `"${VAR:?}"` |
| may touch | `DECISIONS.md` (entry 13's paragraph and tail only), `CLAUDE.md` (its generated block only, by `agentik decisions render`), `design/packets/ar100-entry13.md`, `history/sittings/*.md`, `history/sessions/NEXT` |
| acceptance | `grep -c -F 'verb is a second driver over the same queue files' DECISIONS.md` reads `1` (main: `0`); `grep -c -F 'hq entry 14' DECISIONS.md` reads `1` (main: `0`); `grep -c -F 'amended 2026-11-08, hq entry 14' DECISIONS.md` reads `1` (main: `0`); `grep -c -F 'is the only driver.' DECISIONS.md` reads `0` (main: `1`); `grep -c -E '^[0-9]+\. \*\*' DECISIONS.md` reads `16` (main: `16`); `grep -c '^- 13 The relay: one child, one worktree, one branch (The night)$' CLAUDE.md` reads `1` (main: `1`) |
| budget | 30 turns, 100k context, `claude-opus-5-5`; one act, one commit; **stop at 100k context or 30 tool calls:** commit what is there, write the exact resume line, report; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says

hq entry 14 (2026-11-08, the owner's word): through 2026-12-31 the suite has two drivers over the same queue files, the same packets and the same gate, the Go relay and agentik-session's `unattended` verb; the trial's bar and rule are hq design/07 §6; the relay's next packet amends its entry 13 by one sentence citing hq entry 14. This is that packet, and the first row the loop runs, as hq design/07 §7 asks: the entry is written before any run beyond it. **Not here:** any Go code, any test, any other entry, any change to the queue file, the relay's own `plan` or `run`.

Read first, by sections: this repo's `DECISIONS.md` entry 13 (`agentik decisions show 13`); hq `design/07-unattended-loop.md` §6 and §7 (the bar, the rule, the sentence); the research survey `agentik-research/docs/tools/relay.md`, "The approach" and "What has not worked, and what is open" (what a second driver is measured against). Entry 13 today, its last two sentences: "The driver must show a live view: the rows, the acts as commits land, the live line. The Go relay of this repo, run as `agentik relay`, is the only driver."

## 2. Rulings

0. This packet, committed first on hq's side as this repo's packet file; the builder's one commit is the amendment.
1. **Entry 13's paragraph.** Every sentence stays. The last sentence, "The Go relay of this repo, run as `agentik relay`, is the only driver.", becomes: "The Go relay of this repo, run as `agentik relay`, is the driver, with one exception through 2026-12-31: agentik-session's `unattended` verb is a second driver over the same queue files, for the trial hq design/07 §6 names and hq entry 14 records, and the trial's verdict says which driver remains." No other word of the paragraph changes.
2. **Entry 13's tail.** Inside its closing parenthesis, after the last amendment, append: "; amended 2026-11-08, hq entry 14, the owner's answers to hq questions 14 to 16 of 2026-11-08 (packet AR100)". The date and the title do not change.
3. **The render.** `agentik decisions render` in the worktree rewrites `CLAUDE.md`'s generated block; entry 13's one-line title is unchanged, so the block should read as main does. `agentik decisions render --check` exits 0 before the commit. The edit is by hand (the tool's `answer --amends` takes an open question, and this entry's new fact is hq's answer, not one of this ledger's).
4. **The lint.** `agentik decisions render --lint` exits 0; the paragraph is one paragraph, plain technical English, no quotation of the owner (hq entries 7 and 8).
5. One commit, message "AR100: entry 13 names agentik-session's unattended verb as the trial's second driver (hq entry 14)", by explicit path (`DECISIONS.md`, `CLAUDE.md` if the render changed it, the charter), never `-A`.

## 3. Interfaces

No code. `DECISIONS.md` entry 13 reads as ruling 1 and 2 say; `CLAUDE.md`'s generated block stays byte-identical to main unless `render` changes it, in which case the commit carries it. No test, binary, plugin version or README changes: `plugin-version-check` should ask for nothing, since no file under `plugin/` changes; if it asks, stop and report it rather than bump.

## 4. Findings that name this row

The stop: `agentik decisions render --check` red after the edit (report the lint's line, commit nothing); entry 13 not reading as §1 quotes it (main moved; report, do not guess); `plugin-version-check` asking for a bump. A stop is a result, the row red or built on its branch, the loop's record says which.

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or night record; transcripts; `history/NICE-2026-11-01.md` whole (grep, then line ranges); the Go sources (nothing here needs them).

## 6. Report

At most six lines: the branch and sha; the one commit; the old and the new last sentence of entry 13; `render --check` and `render --lint` exit codes; whether `CLAUDE.md` changed; if stopped, the exact resume line.
