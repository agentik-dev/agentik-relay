# Packet AR74: the child's env names its model, and the report reads a row's last sitting

| item | value |
|---|---|
| from | the observer's check-in 102 (example-mac `history/observer/2026-10-12.md`, F14 and F15); `design/relay-r1.md` §7 (the child's session budget), §9 (`report`), §11; the session plugin's packet S28 (its reader) |
| pin | agent-relay `main` at this packet's commit; worktree `.claude/worktrees/ar74-model-report`, branch `ar74-model-report` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet ./internal/effects ./internal/night`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/effects/spawn.go` (`ChildEnv`, its comment and the session constants only), `internal/night/run.go` (the `ChildEnv` call only), `internal/night/report.go` (`BuildReport` only), the seven `internal/effects/*_test.go` files that call `ChildEnv` (that argument only), `internal/effects/ar74_test.go` (new), `internal/night/ar74_test.go` (new), `design/relay-r1.md` (§7's child-budget paragraph, §9's `report` row, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/effects -run "^Test(AR74_\|AR14_3_\|AR41_1_)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR74_\|ok )"` reads 2; `go test ./internal/night -run "^Test(AR74_\|AR14_3_\|AR41_1_\|AR56_\|AR62_)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR74_\|ok )"` reads 4 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

§7: "After the `CLAUDE*` drop `ChildEnv` adds `CLAUDE_SESSION_DEPTH=1` and `CLAUDE_SESSION_MAX_TURNS=<turns>` … so the session plugin's meter counts the child against its row's ceiling; a parent's values never pass." "a repo that gives its rows a wider budget sets it in its own session config, which outranks the role." §9, `report`: "burn per sitting (builders included), branches waiting on a hand merge, the resume lines, the charters' handover sections".

What happened (F14): every relay child of 2026-10-12 resolved, in the session plugin's `resolved.json`, `session_model: null`, `window_source: declared:opus`, ceiling 180000. wsbind2 ran `claude-opus-5-5[1m]` with `context: 400000` (example-app's queue; its own `ceilings_1m` is 400k), and its handover reads "The session hook's builder profile meters 180k and called EXIT near 185k". The relay spawns `claude -p` (`SpawnArgv`, `internal/night/plan.go:997`), and under `-p` the SessionStart payload carries no `model`, so the plugin falls back to the profile's `opus`, a 200k window, and the repo's 1M ceiling never applies.

What happened (F15): `relay report --night 2026-10-12-3` printed `- wsbind1b: failed, usage-limit` and a Resume block `relay resume wsbind1b`, for a row whose second sitting went green (`status` reads it green, resume ""), and printed the handover `history/sittings/2026-10-12-10-wsbind1.md` twice. In `BuildReport` (`internal/night/report.go:142-156`) each `sitting` line appends to `Stopped.Rows`, `Resume` and `green`, and none clears an earlier sitting's; the handover loop (`:201-222`) runs once per sitting in `r.Sittings`, so a row of two sittings gives each path twice.

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar74.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message.

1. **Act 1, the child's model.** `effects.SessionModelVar = "CLAUDE_SESSION_MODEL"`. `ChildEnv` gains `model string` after `profile`; after the `CLAUDE*` drop it adds `CLAUDE_SESSION_MODEL=<model>`, the spawn line's `--model` exactly as the queue row writes it, marker intact (`claude-opus-5-5[1m]`); "" sets none; a parent's never passes (the drop). `RunRow` passes `flagOf(rp.Spawn, "--model")`. The existing test callers pass "". The variable is inert until the session plugin's S28 reads it; nothing else changes.
2. **Act 2, a row's verdict is its last sitting's.** In `BuildReport` a `sitting` line for a row first removes what an earlier `sitting` line of that row put in `Stopped.Rows`, `Resume` and `green`, then applies its own state as today. A `skip` line is untouched. So a row failed `usage-limit` and then green reads green: no stopped entry, no resume line, waiting once; a row green then red reads red with one resume line and is not waiting; red then red, one entry and one line. `Sittings` (the burn) keeps every sitting; `HandMerges`' clearing (AR56) runs after, unchanged. This is how `status` reads a row.
3. **Act 2, a row's handovers once.** The handover loop reads each row once, at its stage, skipping a row whose last sitting is in flight; rows in first-seen order, paths sorted as today.
4. **Tests.** Effects: `TestAR74_1_ChildEnvCarriesModel` (model `claude-opus-5-5[1m]`: exactly one `CLAUDE_SESSION_MODEL=claude-opus-5-5[1m]`; "": none; a parent's `CLAUDE_SESSION_MODEL=x` never passes). Night: `TestAR74_1_RunPassesModel` (on `TestAR14_3_SpawnEnvCarriesRowTurns`'s world: the child's env holds the queue row's model); `TestAR74_2_LastSitting` (ledger lines, no git: r1 failed `usage-limit` then green, r2 green then red, r3 red then red; r1 has no stopped entry and no resume line and waits once, r2 is stopped red with `relay resume r2` and does not wait, r3 has one entry and one line; the Markdown's Resume block holds `relay resume r2` and `relay resume r3` once each; `Sittings` holds six); `TestAR74_3_HandoverOnce` (on the handover fixture an existing report test uses: a row of two finished sittings on one branch with two dated charters, each path once in `Handovers`).
5. **The record, act 3.** §7's child-budget paragraph: the variable, why (`-p`'s payload has no `model`), and that the session plugin reads it (S28). §9's `report` row: a row's verdict is its last sitting's, and its handovers come once. §11: the four tests.

## 3. Interfaces (signatures, not prose)

`effects`: `SessionModelVar = "CLAUDE_SESSION_MODEL"`; `func ChildEnv(environ []string, row, profile, model string, turns int, remotes []string, bin string) []string`. Red on the pin: `_1` (undefined), `_2` (r1 failed with a resume line), `_3` (a path twice).

## 4. Findings that name this row

The observer's F14 and F15 (check-in 102, 2026-10-12 12:39). The session plugin's packet S28 reads `CLAUDE_SESSION_MODEL`; it merges on its own.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: the acceptance lines' counts, `go vet`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: the TestAR74 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
