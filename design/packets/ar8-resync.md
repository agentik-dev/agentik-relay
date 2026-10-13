# Packet AR8: the re-sync after S8 (the canonical scripts, the pins, pluginlint retired)

| item | value |
|---|---|
| from | conventions §5 (`check`'s order; `validate`) and §6 (the two sha256s, the byte-identical rule); the owner's ruling of 2026-09-23 05:53 (4): the plugin rule lives in the canonical doclint and the charters rule in hygiene, settled in the session's S8 packet; `design/packets/ar6-sync-and-readme.md` §2 items 1 and 2 (the pin; `pluginlint` as the interim home). The builder reads THIS packet and those; never the brief or a ledger |
| pin | agent-relay `main` after `ar7-fixes` merges (cut off `main`; no `base`); worktree `.claude/worktrees/ar8-resync`. Runs only after the session's `s8-conventions-plugins` row merged into its `main` (done 2026-09-23 06:23 at 27b22f9; the orchestrator's order) |
| wall | this repo only, plus two reads outside it: `scripts/hygiene` and `scripts/doclint` of the `claude-code-session` checkout beside this repo's root, on its `main` with S8 merged (its log names the merge), copied with `cp`; nothing else there. No push, `--amend`, rebase, device or network; no `bin/relay`; `make readme` never; nothing under `cmd/`, `internal/`, `plugin/`, `design/` |
| may touch | `scripts/hygiene`, `scripts/doclint`, `scripts/pluginlint` (removed), `Makefile`, `.hygiene-allow`. Nothing else; the charter is admitted by §6 |
| acceptance | `make check; echo $?` reads 0; `cmp -s scripts/hygiene ../claude-code-session/scripts/hygiene; echo $?` reads 0; `cmp -s scripts/doclint ../claude-code-session/scripts/doclint; echo $?` reads 0; `grep -c "^HYGIENE_SHA := $(shasum -a 256 scripts/hygiene \| cut -c1-64)$" Makefile` reads 1; `grep -c "^DOCLINT_SHA := $(shasum -a 256 scripts/doclint \| cut -c1-64)$" Makefile` reads 1; `test -e scripts/pluginlint; echo $?` reads 1; `grep -c pluginlint Makefile` reads 0; `make -n validate \| grep -c doclint` reads 1; `scripts/hygiene; echo $?` reads 0; `git log --oneline main..HEAD -- scripts Makefile .hygiene-allow \| wc -l` reads 1; `git status --short \| wc -l` reads 0 after the last commit |
| budget | 40 turns, 130k context, `claude-opus-5-5`; one commit for the sync (the charter is its own); checkpoint at 80%; the charter is the first free `history/sittings/<date>-<n>.md` on the branch |

## 1. What the source says (quoted)

Conventions §6: "Every copy of `scripts/hygiene` has sha256 `fb05f620...`, of `scripts/doclint` `b46db3df...`" (the values S8 renews). The ruling of 2026-09-23 05:53 (4): "the four conventions questions (the plugin rule in the canonical doclint, ... the two-line sidecar) are settled inside conventions as recommended, one S8 packet". AR6 §2 item 1: "The byte-identity gate this repo lacked is the pin: `HYGIENE_SHA` and `DOCLINT_SHA` in the Makefile"; item 2: "The `plugin` rule survives as `scripts/pluginlint` ... the `validate` target runs it after `claude plugin validate plugin/`". ar4's and ar5's handovers: the charters' `.hygiene-allow` row is `history/sittings/*.md: sibling`.

## 2. Rulings

The owner's standing word: decide inside the design, record each choice in the charter, escalate at most ONE real fork. One commit, in this order inside it:

1. Both scripts become the session's bytes (`cp` from the checkout beside this root); `HYGIENE_SHA` and `DOCLINT_SHA` move to the new sha256s in the same commit, so `make doclint` and `make hygiene` never see a pin and a script from different days.
2. `scripts/pluginlint` is removed (`git rm`); `validate` runs the canonical doclint's plugin rule by the door S8 gave it (its usage line: a rule flag, or the rule inside the default run), after `claude plugin validate plugin/`, so §5's steps keep their order. If the canonical doclint runs the plugin rule with no flag, `validate`'s second line is `scripts/doclint` and the acceptance's `make -n validate` count still reads 1.
3. `.hygiene-allow`: the row `history/sittings/*.md: sibling` is removed and `scripts/hygiene` re-run; a hit brings the row back with a comment naming the canonical rule's gap, and the charter says so. Rows that excuse other paths stay.
4. Nothing else moves: no rendered fact changes, `make readme` is not run, the sidecar stands.

## 3. Interfaces (signatures, not prose)

- Makefile: `HYGIENE_SHA := <64 hex>` and `DOCLINT_SHA := <64 hex>` equal to `shasum -a 256` of the copied scripts; `validate:` two lines, the second naming `scripts/doclint`; `check`'s order unchanged.
- `scripts/doclint` and `scripts/hygiene`: the session's bytes, whatever they are; no local edit, ever (conventions §6).
- No Go, no plugin file, no design file.

## 4. Findings that name this row

Baseline on `main` at b79c2c7, 2026-09-23, S8 merged in the session at 27b22f9 (06:23): both `cmp` lines 1 (this repo's copies are still S7's, `fb05f620`/`b46db3df`; the session's now read `5c7408dd`/`ca28e426`, its doclint naming `plugin` and its hygiene the charters); both pin greps 1 (self-consistent with the old copies); `pluginlint` exists (0) and the Makefile names it once; `make -n validate` names doclint 0 times; `scripts/hygiene` 0; the sync commit count 0; `.hygiene-allow` names `history/sittings` once. `make validate` runs `claude plugin validate plugin/` then `scripts/pluginlint` (`Makefile:22-24`). No Go file names `pluginlint`. Hygiene scans tracked files only (`git ls-files`): commit, then run `make check`.

## 5. Forbidden reads

Every other repo beyond the two files the wall admits; `~/.claude/projects/`; `~/.local/state/agent-relay/`; any ledger; the brief; the relay ledgers under example-mac's `history/relay/`; `design/relay-r1.md`; `design/review-ar2.md`; `.claude/worktrees/`; any file over 8 KB whole unless in `may touch`.

## 6. Report (capped: one table, at most five bullets, under 1,500 words)

The table: each acceptance line, its exit and last line on the branch. Bullets: the two sha256s before and after; the door `validate` uses for the plugin rule; the `.hygiene-allow` outcome; what was left undone and where it goes; the one open fork, if any, with its consumer. Last line: the branch and its head sha.
