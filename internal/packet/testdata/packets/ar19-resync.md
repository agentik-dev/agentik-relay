# Packet AR19: the post-S13 re-sync: the session's hygiene and bounded, doclint held at its pin, the README at 840 s

| item | value |
|---|---|
| from | the session's `record design/conventions.md --section 5,6` (`claude-code-session`, beside this checkout, read only) and its `scripts/hygiene`, `scripts/bounded`, `scripts/doclint` (copied or measured from a temp tree, never run there); `design/readme/source.md`, `design/readme/contract.txt`; the code this packet cites |
| pin | agent-relay `main` after `packets-37-relay` merged (cut off `main`, no `base`); worktree `.claude/worktrees/ar19-resync` |
| wall | this repo only; the session is read, never written or tested. No push, `--amend`, rebase, device or network but `make readme`'s one claude call; every test path under `t.TempDir()`; no `relay plan` or `relay run` by hand; `make e2e` and `make tla` never; `go test ./...` only inside `make check`, at most twice |
| may touch | `scripts/hygiene`, `scripts/bounded`, `Makefile`, `.hygiene-allow` (only if hygiene asks; measured: it does not), `internal/night/ar17c_test.go` (a comment naming the bounded sha, only if stale), `design/readme/source.md`, `README.md`, `design/readme/rendered.sha256`, `history/sittings/*.md`, `history/sessions/NEXT` (removed if present). Nothing else |
| acceptance | `git show HEAD:scripts/hygiene \| shasum -a 256 \| cut -c1-8` reads `d63d0ffb`; `git show HEAD:scripts/bounded \| shasum -a 256 \| cut -c1-8` reads `e343b094`; `git show HEAD:scripts/doclint \| shasum -a 256 \| cut -c1-8` reads `3040666c`; `git show HEAD:Makefile \| grep -c 'd63d0ffb3775b3e4'` reads 1; `make doclint hygiene >/dev/null 2>&1; echo $?` reads 0; `go test ./internal/night -run TestAR17C_3 -count=1 2>&1 \| cut -c1-2 \| tail -1` reads `ok`; `cat README.md design/readme/source.md \| grep -c '900 s'` reads 0; `grep -c '840 s wall' README.md` reads 1; `shasum -c --status design/readme/rendered.sha256; echo $?` reads 0; `grep -c "$(grep -rhc '^func Test' internal cmd \| paste -sd+ - \| bc). Go tests" design/readme/source.md` reads 1; `grep -rl '(filled at close)' history/sittings \| wc -l` reads 0; `test -e history/sessions/NEXT; echo $?` reads 1 |
| budget | 50 turns, 120k context, `claude-opus-5-5` (70 turns with a reason in the charter); one commit per act (1, 2) plus the charter's; checkpoint at 80%; the charter is the first free `history/sittings/2026-09-24-<n>-ar19.md`, opened with doclint's five rows (goal, budget, pin, window, agents), under 4,096 bytes, naming no home path, closed before the last commit |
| verify | `make check` |

## 1. What the source says (quoted)

The session's s13b handover: "for packets-37: bounded `e343b094`, hygiene `d63d0ffb`, doclint `d5957c6c`". Conventions §5: "`check` runs under `scripts/bounded --lock .check.lock --wall 840 --load 4` ... The wall includes the load wait (the relay allows 900 s)". Conventions §6: the three scripts are byte-identical copies, their bytes pinned. The README contract: "keep byte-identical and in their order ... every number, every list item ...; add no fact and drop no fact."

## 2. Rulings

Decide inside the design; record each choice in the charter's handover with its consumer; at most ONE fork.

1. **The re-sync, ONE commit.** `scripts/hygiene` and `scripts/bounded` byte-identical to the session's `main` (`35b5870` or later; the shas in §4, re-measured with `shasum -a 256` before copying; a different sha is a stop, named in the charter); `HYGIENE_SHA` moved; `check` stays `--wall 840`. **`scripts/doclint` and `DOCLINT_SHA` stay at `3040666c`**: the canonical `d5957c6c` refuses two live post-AR10 packets on false positives (§4), no historical packet is edited to satisfy a doclint, and a script that reads red cannot land at a commit. Holding the previous canonical copy keeps §6 (a pinned canonical copy) and is the coordinator's decision, not a fork. Before committing, the builder re-runs the session's doclint over a temp copy of the branch (`git archive HEAD` into `mktemp -d`, `git init`, the script copied in, `python3 scripts/doclint`) and records its exit and lines in the report; if it now exits 0 (the session fixed it first), take it in the same commit, move `DOCLINT_SHA`, and read the doclint acceptance line against the new sha.
2. **The README at 840 s, last act.** `design/readme/source.md` line 90 says `an 840 s wall` where it says `a 900 s wall`; nothing else in the source changes by hand. The builder runs `make readme` once, at close (it rewrites the Status line with today and the live Go test count, renders through `/respeak:respeak technical` under the contract, and stamps the two-line sidecar). Verify the render against the contract before committing: every heading, fenced block, code span, link, number, list item and table row of the source present byte-identical and in order in `README.md` (a scripted comparison into `mktemp`, its count in the report). A render that breaks the contract is fixed by hand to the contract's letter (never a second `make readme`), then `make readme-stamp`; say which in the report. Source, README and sidecar land in one commit.

## 3. Interfaces (signatures, not prose)

None change. `make readme` and `make readme-stamp` are used as written.

## 4. Findings that name this row

Measured on `main` cb2e726: the shas read `efc59152 3040666c 63cebd3d` (session: `d63d0ffb d5957c6c e343b094`). From a temp tree with the session's three scripts: hygiene 0 hits outside the allow file (exit 0); `TestAR17C_`, `TestAR18_` (night), `TestAR17B_` (hooks) and `TestAR18_3_` (packet) all `ok` with the new copies. The canonical doclint exits 2 on two packets the Go parser passes (`TestAR17C_0_LivePacketsParse`): `ar10-bounded-night.md`, the line `t=$(mktemp -d) && RELAY_MEASURE_DEPTH=1 XDG_STATE_HOME="$t" go run ./cmd/relay plan --rows ar10-bounded-night > "$t/o" 2>&1` (the assignment's double-quoted value is split off, so its placeholder is read as a non-literal command word, although the line has `--rows`); and `ar15-readme-r7.md`, the expected value `` `<img src="assets/icon.svg" alt="" width="56" align="left">` `` after `reads` (doclint reads every code span of the cell as a line; the Go parser splits each item at `reads` and scans only the shell side). README and source line 90 say `a 900 s wall`; the source's Status line says `253` Go tests, the live count is 306; the sidecar is fresh.

## 5. Forbidden reads

Every other repo but the session's three scripts and conventions; `~/.claude/projects/`; any ledger or transcript; the real state root; `.claude/worktrees/` but your own; the historical packets (read none; the two lines above are quoted whole).

## 6. Report (capped)

The table: each acceptance line, its exit and last line. Bullets: the session's doclint re-measured at build (exit and lines); the render's contract comparison (count kept, count broken, how fixed); the README's test count; the one open fork. Last line: the branch and its head sha.
