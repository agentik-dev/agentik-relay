# Packet AR17D: run 10's rulings for the relay: the account stop, the verify row, the named signal, repo-relative rows (rulings 1 to 4, 6 to 9, 14; ADV10-8, -9, -19, -22, -24)

| item | value |
|---|---|
| from | `design/review-adv10.md` whole; example-mac's `history/relay/2026-09-23-5-decisions.md` rows 1 to 9 and 14 only; `design/relay-r1.md` §1, §3, §9, §11, §13, §14, §16; `design/tla/`; the code the review cites. Never a ledger but a fixture's |
| pin | agent-relay `main` after `ar17c-bounds-r10` merged (cut off `main`, no `base`); worktree `.claude/worktrees/ar17d-rulings-r10` |
| wall | this repo only. No push, `--amend`, rebase, device or network; every test path under `t.TempDir()`, never the real queue, ledger or state root; a hand `relay plan` only with `--rows`, `RELAY_MEASURE_DEPTH=1` and a temp `XDG_STATE_HOME`; `go test ./...` only inside `make check` |
| may touch | `internal/night/`, `internal/relay/`, `internal/stream/`, `internal/effects/`, `internal/queue/`, `internal/packet/`, `internal/gate/`, `cmd/relay/`, `design/relay-r1.md`, `design/tla/Night.tla`, `design/tla/Night.cfg`, `design/packets/rows-*.json`, `.hygiene-allow`, `history/sittings/*.md`, `history/sessions/NEXT` (removed if present). Nothing else |
| acceptance | `for p in night relay stream effects queue packet gate; do go test ./internal/$p -run TestAR17D_ -count=1 >/dev/null 2>&1 \|\| exit 1; done; echo ok` reads `ok`; `go test ./cmd/relay -run TestAR17D_ -count=1 2>&1 \| cut -c1-2 \| tail -1` reads `ok`; `go test -tags e2e ./cmd/relay -run TestE2E_10_ -count=1 2>&1 \| cut -c1-2 \| tail -1` reads `ok`; `grep -rhc '^func TestAR17D_' internal cmd \| paste -sd+ - \| bc` reads 9 or more; `grep -c 'recommended, unruled' design/relay-r1.md` reads 0; `grep -c 'run 10 ruling' design/relay-r1.md` reads 5 or more; `grep -l '/Users/' design/packets/rows-*.json \| wc -l` reads 0; `grep -c 'rows-\*.json: home' .hygiene-allow` reads 0; `grep -rl '(filled at close)' history/sittings \| wc -l` reads 0; `test -e history/sessions/NEXT; echo $?` reads 1 |
| budget | 80 turns, 130k context, `claude-opus-5-5`; one commit per act (0 to 5) plus the charter's; `make check` is the verify line, never acceptance; checkpoint at 80%; the charter is the first free `history/sittings/2026-09-23-<n>-ar17d.md`, closed before the last commit |
| verify | `make check` |

## 1. What the source says (quoted)

The owner, 2026-09-23: "I accept your recommendations". Ruling 1: "A usage limit or an expired login now stops the whole run with its own stop reason, not just the one row ... The failing child's uncommitted files are kept for the next attempt." The run-10 prompt: "the Go relay's stop reason for an account failure joins the §1 list, with a test through the fake claude (a result text knob)."

## 2. Rulings

Build the rulings as written; decide inside the design, record each choice in the charter, at most ONE fork. One commit per act; each act's test reproduces its gap FIRST (red on `main`), then the fix. Act 0 changes `Night.tla`; `make tla` reads `tla: ok` once before the closing commit (reported).

0. **The account stop (ruling 1; ADV10-9).** Two stop reasons join §1's list (twenty-three): `usage-limit` and `auth`, both sitting failures. `stream` classifies a `rate_limit_event` whose status is not allowed, and a result with `is_error` whose text names a usage or rate limit (`usage-limit`) or a login, an expired token, `401` or `/login` (`auth`); the patterns live in one table in `stream`. On either, before the gate: the row is `failed` with that reason, every other running row is killed (TERM, `KillGrace`, KILL) and reads `failed` with the same reason, no new spawn, `relay run` exits 1; no worktree is removed or cleaned, and `relay clean --stale` skips a worktree whose last exit line carries either reason. The fake claude gains `FAKECLAUDE_RESULT` (the result text, `is_error` true, per row like `FAKECLAUDE_MODE`). `TestAR17D_0_UsageLimitStopsNight`, `TestAR17D_0_AuthStopsNight`, `TestAR17D_0_ParallelRowsKilled`, `TestAR17D_0_UncommittedKept`; `TestE2E_10_UsageLimit` in `e2e_test.go` through the built fake. Model: `AccountFails` stops the night; `NoSpawnAfterAccountStop` in `Night.cfg`.
1. **The verify row is read (ruling 6; ADV10-24).** `relay plan` refuses, as an invalid row (exit 2, nothing measured, D2 §1), a packet whose `verify` row differs from the resolved gate line: `packet verify row \`X\` differs from the gate line \`Y\``. `TestAR17D_1_VerifyRowMismatchRefused`, `TestAR17D_1_VerifyRowMatchPlans`.
2. **The named signal (ruling 7; not taken by AR13 or AR14: `run.go:365` cuts `signal: killed` to `signal`).** The exit line's `error` keeps a signal whole (`signal: killed`); an exit status stays `exit N` only (§5: stderr never follows). `TestAR17D_2_SignalNamedOnExit`.
3. **Repo-relative rows (ruling 8; ADV10-8).** Every tracked `design/packets/rows-*.json` gets `repo` as a bare name (`agent-relay`) and `prompt` repo-relative (`design/packets/<file>`), nothing else changed; `.hygiene-allow` drops `home` from the rows line. relay-r1 §13 states the rows schema: a bare `repo` resolves to the checkout beside the one holding the rows file, `prompt` under it; the reader is the stand-in's (the Go relay reads no rows file), and its bare-name resolver is the orchestrator's, landed before this branch merges. `TestAR17D_3_RowsFilesRepoRelative` reads every rows file (copies, under `t.TempDir()`) and fails on an absolute or `~` path.
4. **Ids and the real checkout (ADV10-19, -22).** A row id must pass `git check-ref-format --branch` and is never the base, `HEAD`, or ends `.lock`: `TestAR17D_4_RefShapedIds`. `TestAR10_0_NoTestOpensRealCheckout` scans every `_test.go` under `cmd` and `internal`; `gate_test.go:337` and `packet/ar10_test.go:61` read copies: `TestAR17D_4_NoTestReadsRealTree`.
5. **The design (rulings 2, 3, 4, 9, 14).** relay-r1: §1's rule line cites `D19, run 10 ruling 2` in place of "recommended, unruled"; §14's D19, D20, D21 lines each end `ruled by the owner 2026-09-23 (run 10 ruling 2|3|4)`; §9's exit codes cite ruling 4; §1 lists `usage-limit` and `auth`; §11 and §16: "an e2e sitting and every adversarial review run `make tla` beside `make e2e` and report both last lines; `make tla` stays outside `check` (JVM, jar; run 10 ruling 14)". Charter `history/sittings/2026-09-23-4.md`: its two `(filled at close)` placeholders become `closed without an exit line (run 10 ruling 9, 2026-09-23)`, nothing else in it changed.

## 3. Interfaces (signatures, not prose)

- `relay.UsageLimit`, `relay.Auth StopReason`; `stream.Account(ev map[string]any) relay.StopReason` (`""` when neither); `night.Plan` row problem `verify-row`; the fake's `FAKECLAUDE_RESULT`.

## 4. Findings that name this row

Baseline on `main` after ar17c merged: `grep -c 'recommended, unruled' design/relay-r1.md` reads 1; 11 rows files hold `/Users/`; charter 4 holds two placeholders; ADV10-19 at `queue.go:341`.

## 5. Forbidden reads

Every other repo but the one decisions file named in `from`; `~/.claude/projects/`; the real state root; any ledger but a fixture's; the packets before `ar13-*` (the rows files excepted, for act 3); `.claude/worktrees/` but your own.

## 6. Report (capped)

The table: each acceptance line, its exit and last line. Bullets: per act the reproducing test's red line on `main`; `make tla`'s last line; what was left undone; the one open fork. Last line: the branch and its head sha.
