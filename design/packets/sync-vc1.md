# Packet sync-vc1: plugin-version-check vendored and wired into check, the plugin at 0.2.0

| item | value |
|---|---|
| from | agentik-commons `GUIDE.md` §5 and §7; commons' `scripts/sync`, whose fourth copy is `scripts/plugin-version-check`; agentik-hq `HANDOFF.md` items 3 and 5 (each plugin repo wires the check into its `check` in its next sync packet); the bump `0.1.3` to `0.2.0`, decided by hq sitting 2026-10-17-2-hq |
| pin | `main` at `32f9463` plus this packet's own commit; agentik-commons `main` at `02409f0`; branch `sync-vc1`, worktree `.claude/worktrees/sync-vc1`, cut from main; a hand-run builder |
| wall | AR95's: this repo only; agentik-commons read-only (its `scripts/sync` writes here, once). No push, merge, `--amend`, rebase, `git stash`, network or install; no `claude` beyond `make check`'s `validate`; never build or run a relay binary; never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`; never write inside `~/.local/state/agent-relay`, `.venv` or `~/.config`. No `make test`, `make install`, `make readme`, `make readme-status`, `make e2e` or `go test ./...`: the acceptance lines, `make doclint` and `make hygiene`, one at a time at `nice -n 10`, each shell line with `PYTHONDONTWRITEBYTECODE=1`. One `make check`, last, under ruling 5 |
| may touch | `scripts/plugin-version-check` (new) and `scripts/vendored.sha256` (by agentik-commons' `scripts/sync` only), `scripts/agentik`, `scripts/hygiene`, `scripts/bounded` (by the sync only, and only if commons' bytes moved: at the pins they are equal), `Makefile` (the target and its name in the chain), `plugin/.claude-plugin/plugin.json` (its `version` only), `internal/hygiene/agentik_r1_test.go` (`TestAgentikR1_2_VendoredPinned`: its name list, its two counts and its comment, three to four, only), `internal/packet/agentik_r1_test.go` (its three `0.1.3`, lines 13, 23 and 25, only), `design/packets/sync-vc1.md`, `history/sittings/*.md`, `history/sessions/NEXT`. Nothing else |
| acceptance | `grep -c -F 'c97596e7b129ac53fb2c8b22adfcf3a9d7e5bd71b6c9473d719aa2cda8063c4b  scripts/plugin-version-check' scripts/vendored.sha256` reads `1` (main: `0`); `grep -c -F -e '"bounded", "plugin-version-check"' -e 'ok != 4 ' -e 'of 4 OK' internal/hygiene/agentik_r1_test.go` reads `3` (main: `0`); `sed -n '/^hygiene:/,/^validate:/p' Makefile \| grep -c -F 'PY) -B scripts/plugin-version-check'` reads `1` (main: `0`); `grep -c -F 'scripts/plugin-version-check' Makefile` reads `1` (main: `0`); `grep -c -F '"version": "0.2.0"' plugin/.claude-plugin/plugin.json` reads `1` (main: `0`); `grep -c -F '0.2.0' internal/packet/agentik_r1_test.go` reads `3` (main: `0`); `sed -n '/^check-unlocked:/p' Makefile \| grep -c plugin-version-check` reads `1` (main: `0`) |
| budget | 25 turns, 100k context, `claude-opus-5-5`; three acts, one commit each; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says

- GUIDE §5: `check` runs `shasum -a 256 -c scripts/vendored.sha256`, then `scripts/plugin-version-check`, which "fails when a file under `plugin/` changed after the commit that last changed the version (a bump not yet committed passes)" (§7; `0.x` may break on a minor). It exits 0 clean, 1 with one stderr line per file, 2 usage.
- Commons' `scripts/sync <repo>` copies `bin/agentik` to `scripts/agentik` and its `hygiene`, `bounded` and `plugin-version-check` into `scripts/`, then writes `scripts/vendored.sha256` (four lines); nothing else, no commit. At the pins the first three equal commons' bytes (`cmp` 0, three `OK`), so the sync changes two files: `scripts/plugin-version-check` (new) and `scripts/vendored.sha256` (one line added).
- The check on main exits 1 with nine lines, each `changed since d5c52b4e0904 set version 0.1.3`: `plugin/.claude-plugin/plugin.json`, `plugin/agents/attendant.md`, six of `plugin/commands/` (`doctor`, `gate`, `plan`, `report`, `resume`, `status`), `plugin/hooks/run`; all by the merge `a166166` of agentik-r1-relay, whose act 5 (`72c0fe0`) made the plugin `relay@agentik`, its verbs `agentik relay`.
- Two tests pin main. `TestAgentikR1_2_VendoredPinned` (`internal/hygiene/agentik_r1_test.go`) wants exactly three `OK` lines: red after the sync (`read 3 of 3 OK lines`, four printed). `TestAgentikR1_5_PluginIsRelay` (`internal/packet/agentik_r1_test.go`) pins `0.1.3` at lines 13, 23 and 25: red on the bump.
- `check-unlocked: test vet fmt doclint hygiene validate`; `hygiene`'s recipe runs the sha check, then `$(PY) -B scripts/hygiene` (`-B`: `PYTHONDONTWRITEBYTECODE=1` as a flag).
- Simulated in a scratch clone at the pins (rulings 1 to 3, committed): `make hygiene` reads `hits outside allowlist: 0`, then `version 0.2.0, set at <sha>; nothing under plugin/ changed since`; `make doclint` exits 0; both tests green once moved.

## 2. Rulings

Standing rules: the charter, `history/sittings/<date>-<n>-sync-vc1.md` (`n` the day's next), opens at act 1 and is committed with act 3; no home path; the commits of acts 2 and 3 quote their red line.

1. **The sync is the only writer** of the vendored files; none is edited by hand. It runs once, from agentik-commons, with this worktree as its argument. A third changed file under `scripts/` means commons moved past `02409f0`: report it, edit nothing. `TestAgentikR1_2_VendoredPinned` moves with it: `"plugin-version-check"` joins its list, `3` becomes `4` in its two counts and message, "three" becomes "four" twice in its comment.
2. **The wiring.** The Makefile gains a target `plugin-version-check` between `hygiene` and `validate`: a `##` help line and one recipe line, a tab then `$(PY) -B scripts/plugin-version-check`; `check-unlocked`'s prerequisite list names `plugin-version-check` right after `hygiene`. `hygiene`'s help line, its recipe and `make hygiene` are unchanged (§4); no help text or comment spells `scripts/plugin-version-check`, so acceptance line 4 reads as measured.
3. **The bump.** `plugin/.claude-plugin/plugin.json`'s `version` becomes `0.2.0` (0.x, a minor, since the plugin's verbs renamed), in the same commit as ruling 2's line; nothing else in it moves. The three `0.1.3` of `internal/packet/agentik_r1_test.go` become `0.2.0` in that commit, nothing else of the test. No tag: the tag is the owner's. After that commit `scripts/plugin-version-check` exits 0.
4. **`.hygiene-allow` is not edited.** A new hygiene hit is red: report its group and count and leave the branch as it is (the simulation read 0).
5. **Commits by explicit path**, `git add <paths>` then `git commit` (`git commit <path>` refuses an untracked file). Act 1: this packet alone, verbatim (entry 2's copy-in). Act 2: the sync, its two files and ruling 1's test pin; red line, acceptance line 1 reading `0`. Act 3: rulings 2 and 3, the charter; red line, read after act 2, `scripts/plugin-version-check; echo rc=$?` reading `rc=1` and nine lines. One `make check`, last, in the worktree, only when `ps -axo command | grep -E '(^|/)make (check|check-unlocked)( |$)' | grep -v grep` prints nothing (sibling repos build this packet today; a night's `make check-fast` does not hold it), read every 20 to 60 s with jitter, up to 20 minutes, else unrun. A last stderr line `bounded: stop: lock`, `load` or `wall` is unrun, not red: wait 60 s and run it once more.
6. **The list is closed.** The README's Status line (`README.md`, `design/readme/source.md`, line 105, `0.1.3`) stays: `make readme-status` and `make readme` are outside the wall, and the freshness check's sums do not move.

## 3. Interfaces

No Go code changes. `make hygiene`, and so `make check`, gain one step, `scripts/plugin-version-check` (exits 0, 1, 2); a red one stops `check-unlocked` before `validate`. `scripts/vendored.sha256` holds four lines. The plugin is `relay` at `0.2.0`, all else as on main.

## 4. Findings that name this row

agentik-hq `HANDOFF.md` items 3 and 5, answered for this repo at the merge; the note is hq's. The commons marketplace's `relay` line (`0.1.3`) is not this packet's: the hq sitting's bump helper moves it after the merge, and until then commons' `make check` (`marketplace-versions --if-complete`, against this repo's main checkout) reads it as trailing. For NICE, in the report (`NICE_TO_HAVE.md` is outside the list): the README's Status line keeps `0.1.3` until the next `make readme` (S4); `hygiene`'s help text and sha-check message name three vendored copies, not four (S4).

## 5. Forbidden reads

Transcripts; anything under `~/.local/state` or `~/.claude/projects`; the private hygiene list; any other repo, except agentik-commons' `scripts/sync`, `bin/agentik`, `scripts/hygiene`, `scripts/bounded`, `scripts/plugin-version-check` and `GUIDE.md`.

## 6. Report

At most six lines: each act's sha; the files the sync changed; each acceptance line's result; the hand checks (`shasum -a 256 -c scripts/vendored.sha256`, four lines ending `: OK`; `cmp` of the four copies against commons' exits 0; `scripts/plugin-version-check; echo rc=$?` reads `rc=0` after act 3); `make hygiene`'s `hits outside allowlist` count; `make check` (green; red, with the failing target's last lines; or unrun); §4's NICE leads. Fork: none, or one.
