# Packet AR6: the scripts re-synced with a pin, the README sidecar, and AR2's lows outside ar4 and ar5

| item | value |
|---|---|
| from | conventions §5 (the sidecar; `check`'s order) and §6 (the two sha256s); the brief's items 24 and 26 as ruled 2026-09-22 16:30, restated in §2; `design/review-ar2.md` AR2-14, AR2-17, AR2-23 only; `design/packets/ap1-plugin.md` §3 (the doclint rule `plugin`); the may-touch rows of `design/packets/ar4-fixes.md` and `ar5-ux.md` (the wall on the lows). The builder reads THIS packet and those; never the brief or a ledger |
| pin | agent-relay `main` after `ar5-ux` merges (the relay cuts `ar6-sync-and-readme` off `main`; no `base`); worktree `.claude/worktrees/ar6-sync-and-readme` |
| wall | this repo only, plus two reads outside it: `scripts/hygiene` and `scripts/doclint` of the `claude-code-session` checkout beside this repo's root, on its `main` (`66c6357`), copied with `cp`; nothing else there. No push, `--amend`, rebase, device or network; `bin/relay` never runs without `XDG_STATE_HOME` set to a fresh temp directory; `make readme` at most once, only if a rendered fact changed, counted in the charter; nothing on ar4's or ar5's may-touch lists; `design/relay-r1.md` untouched |
| may touch | `scripts/doclint`, `scripts/hygiene`, `scripts/pluginlint` (new), `Makefile`, `design/readme/rendered.sha256` (new), `design/readme/source.md` (the editor line only), `README.md` (only through make readme), `.hygiene-allow` (a row for the new script only if the scan asks), `internal/effects/notify.go`, `internal/effects/notify_test.go`, `internal/effects/spawn.go`, `internal/effects/spawn_test.go`, `internal/packet/packet.go`, `internal/packet/packet_test.go`, `internal/yaml/yaml.go`, `internal/yaml/yaml_test.go`. Nothing else; the charter is admitted by §6 |
| acceptance | `make check; echo $?` reads 0; `make check 2>&1 \| tail -1` reads `readme: fresh`; `shasum -a 256 scripts/hygiene \| cut -c1-12` reads `fb05f6203ac9`; `shasum -a 256 scripts/doclint \| cut -c1-12` reads `b46db3dfed41`; `grep -Ec 'HYGIENE_SHA\|DOCLINT_SHA' Makefile` reads 4 or more; `grep -c ' -nt ' Makefile` reads 0; `shasum -c --status design/readme/rendered.sha256; echo $?` reads 0; `grep -c 'shasum -c --status design/readme/rendered.sha256' Makefile` reads 1; `scripts/pluginlint; echo $?` reads 0; `t=$(mktemp -d) && git -C "$t" init -q && cp -R plugin "$t"/ && rm "$t"/plugin/commands/doctor.md && scripts/pluginlint --repo "$t"; echo $?` reads 2; `go test ./... -run 'TestAR6_' -v 2>&1 \| grep -c '^--- PASS'` reads 4; `git status --short \| wc -l` reads 0 after the last commit |
| budget | 80 turns, 130k context, `claude-opus-5-5`; 80 over 60 because four of the seven acts are a reproduce-then-fix pair; `make check` carries `-race`; one commit per act (§2); checkpoint at 80%; never push, amend or rebase; the charter is the first free `history/sittings/2026-09-22-<n>.md` on the branch |

## 1. What the source says (quoted)

Conventions §5: "the sidecar `design/readme/rendered.sha256` holds the full `shasum -a 256 design/readme/source.md` line, written by `make readme` and checked from the repo root by `shasum -c --status`". §6: "Every copy of `scripts/hygiene` has sha256 `fb05f620...`, of `scripts/doclint` `b46db3df...`". Item 24 (ruled): "rule the content stamp, canonical shape the session's sidecar ... checked by `shasum -c`". Item 26 (ruled): "re-sync the copies ... keep the byte-identical rule rather than allowing a differing first line". AP1 §3: "`scripts/doclint` rule `plugin`: every `plugin/commands/*.md` under 31 lines with `description` and `allowed-tools` in its frontmatter; every `plugin/agents/*.md` with `name`, `description`, `model`, `tools`; the command set equals §10's six". AR2-17: "`Notices` panics on a row id of 186 bytes or more (notify.go:70)"; "the budget pattern truncates `$15.50` (packet.go:88)"; "two malformed YAML inputs parse (yaml.go:155)". AR2-23: "the stderr tail can start mid-rune (spawn.go:131)". Each is "not reproduced by the coordinator".

## 2. Rulings

The owner's standing word: decide inside the design, record each choice in the charter, escalate at most ONE real fork. One ruling per act:

1. Both scripts become the session's bytes (the shebang `#!/usr/bin/env python3`; doclint without `check_plugin`). The byte-identity gate this repo lacked is the pin: `HYGIENE_SHA` and `DOCLINT_SHA` in the Makefile (respeak's shape, the sibling that already matches), each checked by its own target before the script runs; the failure line names conventions §6.
2. The `plugin` rule survives as `scripts/pluginlint`: stdlib python, doclint's `--repo DIR` shape and exit codes, `check_plugin`, `frontmatter`, their constants and helpers moved verbatim, the docstring the moved paragraph; the `validate` target runs it after `claude plugin validate plugin/`, so §5's five steps keep their order.
3. `check`'s readme line becomes the session's three (`skipped` when the source is absent, `fresh` when `shasum -c --status` passes, else `stale, run make readme` and exit 2); `readme` writes the sidecar as its last line; the source's editor line names the sidecar instead of "older". No rendered fact changes, so `make readme` is not run and the sidecar is written by that same shell line (count 0 in the charter).
4. AR2-17 notify: `Notices` never hands `capText` a negative cap: the suffix is capped to `NoticeCap` first, the body to what remains.
5. AR2-17 budget: `$15.50` is refused with the existing row-budget error (whole dollars); `budgetRE` gains an end boundary after the digits (RE2 has no lookahead: a `(?:[^\d.]|$)` tail or a post-match check).
6. AR2-17 yaml: `depth` below zero refuses `closer without opener` at both sites (a fresh line, `yaml.go:168`; a continuation, `:158`); the `unterminated flow collection` refusal at end of input stays.
7. AR2-23 tail: `tailBuffer.String` drops leading UTF-8 continuation bytes so the tail starts on a rune; `capText` still trims the end.

Named for a later packet, not built: every other low sits on ar4's or ar5's list. AR2-14's four (`schedule.go:41`, whose fix changes `PickIn`'s signature and its caller `night.go:529`; `stage.go:48`; `gate.go:172`; `gate.go:61`); AR2-17's six (`ledger.go:66`, `:91`; `git.go:60`; `effects.go:25`; `queue.go:340`, `:87`); AR2-23's two (`main.go:984`, `:936`; `TestPlanEqualsRun`).

## 3. Interfaces (signatures, not prose)

- Makefile: `HYGIENE_SHA := fb05f620...`, `DOCLINT_SHA := b46db3df...` (the full 64); `hygiene:` and `doclint:` each begin `@echo "$(X_SHA)  scripts/x" \| shasum -a 256 -c --status \|\| { echo "x: scripts/x is not conventions §6's copy"; exit 2; }`; `validate:` two lines; `check:` the three readme lines; `readme:` ends `@shasum -a 256 design/readme/source.md > design/readme/rendered.sha256`.
- `scripts/pluginlint [--repo DIR]`: prints refusals one per line; exit 0 clean, 2 on a refusal, 1 on usage.
- `notify.go`, `spawn.go`, `packet.go`, `yaml.go`: no exported signature changes; `yaml.go` adds the refusal text `closer without opener`.
- Tests, four: `TestAR6_NoticesLongRowID` (notify_test: a 300-byte row id on a red `sitting`; no panic, `Body` at most `NoticeCap`); `TestAR6_BudgetRefusesCents` (packet_test: `$15.50` errs, `$15` reads 15); `TestAR6_YAMLCloserWithoutOpener` (yaml_test: `a: ]`, and `a: [1,` then `]]`, each refused with its line number); `TestAR6_StderrTailStartsOnRune` (spawn_test: 401 bytes of two-byte runes; `utf8.ValidString` of `String()` is true). A low that does not reproduce keeps its test as the pin and the charter says so.

## 4. Findings that name this row

Baseline on `main` at 6f996f8, 2026-09-22: `make check` 0, last line `readme: fresh`; `scripts/hygiene` reads `5232a0eff476`, `scripts/doclint` `469f4c3984ed` (the diff from the session's: the shebang, and `check_plugin` with `frontmatter` and four constants, 60 lines); the session's doclint over this tree exits 0; the pin grep reads 0; ` -nt ` reads 1; the sidecar is absent (its line reads 2); `scripts/pluginlint` reads 127 twice; `TestAR6_` reads 0. The code: `notify.go:70` `capText(..., NoticeCap-len(suffix))`; `packet.go:88` `budgetRE` unanchored after `\$(\d+)`; `yaml.go:158` and `:168` set `depth` with no lower bound, `:170` tests only `> 0`; `spawn.go:131` `Write` keeps the last `StderrCap` bytes by count, `:141` `String`.

## 5. Forbidden reads

Every other repo beyond the two files the wall admits; `~/.claude/projects/`; `~/.local/state/agent-relay/`; any ledger outside `t.TempDir()`; the brief; `design/relay-r1.md`; `design/review-ar2.md` beyond the three entries; `.claude/worktrees/`; any file over 8 KB whole unless in `may touch`.

## 6. Report (capped: one table, at most five bullets, under 1,500 words)

The table: each acceptance line, its exit and its last output line on the branch. Bullets: the Makefile's readme and pin lines before and after; every choice made inside the design (the pin, `pluginlint`, the sidecar, each low's fix or non-reproduction); the lows named for later; the `make readme` count; the one open fork for the owner, if any, with its consumer, and what was left undone. The last line: the branch name and its head sha.
