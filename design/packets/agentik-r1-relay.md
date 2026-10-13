# Packet agentik-r1-relay: agentik-relay, plugin `relay`, guards on every spelling

| item | value |
|---|---|
| from | agentik-commons `design/agentik-r1.md` §4.1, §4.3, §5.1, §5.2, §6.1–§6.3, §8.2, §8.4, §9 (A2, A3, A6), §10 (D2, D3); packet agentik-r1-commons §2–§3 (the vendored files and their pin; `renamed`; exits 0 clean, 1 hit, 2 usage). The builder reads THIS packet and those sections only |
| pin | agentik-relay `main` at this packet's commit; agentik-commons main after agentik-r1-commons merges; worktree `.claude/worktrees/agentik-r1-relay`, branch `agentik-r1-relay` |
| wall | this repo only; commons is read, and its `scripts/sync` runs once, writing here. No push, `--amend`, rebase, device or network; no `claude plugin` command but `make validate`. Build or run no relay binary, under any name, outside `go test`; no relay verb in any spelling. No `make check`, `install`, `build`, `doctor`, `readme`, `e2e` or `go test ./...`: the acceptance lines, package tests, `go vet` per package, `make doclint`, `hygiene`, `validate` and `readme-stamp`, one at a time at `nice -n 10`, each with `PYTHONDONTWRITEBYTECODE=1`. Never write in `~/.local/state`, `~/.config` or `~/.local/bin`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `cmd/relay/` (moved), `cmd/agentik-relay/`, `internal/hooks/`, `internal/effects/`, `internal/night/`, `internal/relay/`, `internal/gate/`, `internal/hygiene/`, `internal/packet/*_test.go`, `internal/prompt/rules-orchestrator.txt`, `plugin/.claude-plugin/plugin.json`, `plugin/commands/*.md`, `plugin/agents/*.md`, `plugin/hooks/run`, `.claude-plugin/marketplace.json` (deleted), `scripts/agentik`, `scripts/hygiene`, `scripts/bounded`, `scripts/vendored.sha256`, `Makefile`, `.hygiene-allow`, `README.md`, `design/readme/`, `DECISIONS.md`, `assets/icon.svg`, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/hooks -run '^Test(AgentikR1_\|AR11_\|AR40_)' -count=1 -v 2>&1 \| grep -c -E '^(--- PASS: TestAgentikR1_\|ok )'` reads 4; `go test ./internal/effects -run '^Test(AgentikR1_\|LoadConfig\|AR59_)' -count=1 -v 2>&1 \| grep -c -E '^(--- PASS: TestAgentikR1_\|ok )'` reads 4; `go test ./internal/night -run '^Test(AgentikR1_\|AR62_\|AR74_)' -count=1 -v 2>&1 \| grep -c -E '^(--- PASS: TestAgentikR1_\|ok )'` reads 5; `go test ./cmd/agentik-relay -run '^Test(AgentikR1_\|AR59_)' -count=1 -v 2>&1 \| grep -c -E '^(--- PASS: TestAgentikR1_\|ok )'` reads 4; `go test ./internal/packet -run '^Test(AgentikR1_\|AR20_\|AR72_)' -count=1 -v 2>&1 \| grep -c -E '^(--- PASS: TestAgentikR1_\|ok )'` reads 2; `go test ./internal/hygiene -run '^Test(AgentikR1_\|PublicR1_)' -count=1 -v 2>&1 \| grep -c -E '^(--- PASS: TestAgentikR1_\|ok )'` reads 3 |
| budget | 140 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

§4.1, §4.3: `agentik-relay` by `go build -o ~/.local/bin/…`; `relay` a symlink to it until 2026-12-31; `~/go/bin/relay` becomes a link; under the old name a terminal gets one stderr note, "Hooks, pipes and gates see nothing"; printed: `agentik relay resume <row>`. §5.1, D2: `relay@agentik` (`/relay:status`, `relay:builder`); plugin code calls `agentik-relay`; "shims try the new name first, then the old". D3: install also installs `agentik`.

§6.3: "Only names change, never formats. Readers take the new path, else the old"; folders move only in the cold window. c1 state `claude-code-session/` to `agentik-session/`; c2 state `agent-relay/` to `agentik-relay/`; c3 `~/.config/agent-relay/` to `~/.config/agentik/relay/`; c4 `.claude/claude-code-session/` to `.claude/agentik/`, "doctor flags the old"; c6 section `agent-relay` to `relay`, "`relay` wins with a warning"; c11 shims, fallback the alias; c12 guards "match all three spellings", the old stays denied; c14 `cmd/relay` is renamed.

§8.2: clear the §6.2 row, vendor `scripts/{agentik,hygiene,bounded}`, delete `marketplace.json`, `renamed` 0. A6: a row is denied `agentik relay run`, `agentik-relay run`, `relay run`, `agentik session start`, `claude-session start`. §8.4: an old session's shim calls `relay hook`; a child keeps its absolute `RELAY_BIN`; nothing installs.

## 2. Rulings

Charter `history/sittings/<date>-<n>-agentik-r1-relay.md`, opened at act 1, closed before the last commit; each act's first test is red on the pin, its red line in the commit. Acts: 1 the guards (c12); 2 vendor the commons files; 3 the paths (c1–c4, c6); 4 the printed lines; 5 `cmd/agentik-relay`, the Makefile, the plugin (c11, c14, D2, D3); 6 the sweep to `renamed` 0.

1. **Scope.** c1 (`internal/effects/config.go:49`) and c14 (`cmd/relay/`) are in §6.2's relay row, not §8.2's; "clears its whole §6.2 row" binds.
2. **Guards.** `relayBin` reads `agentik-relay`, or `agentik` then `relay`, as the relay; `relayPath` matches `cmd/agentik-relay`; the session guard, renamed `nested-session`, matches `^(claude-session|agentik-session|agentik session)(\s|$)`, any verb as today.
3. **New, else old.** Each path (state root, pid dir, the three config layers) is the new one if it exists, else the old if it exists, else the new; the file door denies both spellings of the state root and the pid dir.
4. **Warnings.** c4's old path and c6's two sections go to a new `Config.Warnings`, on which doctor's `config` check warns, never fails.
5. **One resume line.** `relay.ResumeLine` writes it; `relay.ResumeRow` reads it and the old `relay resume <row>` (ledgers and `status.json` from before); every producer and reader uses them.
6. **Alias note.** `main` prints `relay: renamed; use 'agentik relay …' (ends 2026-12-31)` once to stderr when argv[0]'s base is `relay`, stderr is a terminal and the verb is not `hook`.
7. **Pin file.** `make hygiene` checks `scripts/vendored.sha256` with `shasum -a 256 -c` in place of `HYGIENE_SHA`; `DOCLINT_SHA` stays; `internal/hygiene`'s tests swap exits 1 and 2.
8. **Version.** The plugin stays `0.1.3`, so the commons marketplace's entry stays true.
9. **Spelling.** Go comments and plugin command bodies take `agentik-relay <verb>`; README, DECISIONS.md and printed lines `agentik relay <verb>`; a test pinning an old spelling's guard or fallback keeps it.
10. **Records.** `internal/packet/testdata/a*.md` (record copies) and `history/sittings/*.md` (per-packet charters, no launcher chain) get `renamed` rows; each other exception is one row per file under a `#` reason.
11. **README.** README.md and `design/readme/source.md` take the same hand edits, re-pinned by `make readme-stamp`; `make readme` never runs.
12. **`renamed` 0** is no `renamed` entry in `--json`'s outside-allowlist `files`; the group's counts include allowed files.

## 3. Interfaces (signatures, not prose)

`relay`: `ResumeLine(row string) string`, `ResumeRow(line string) (row string, ok bool)`. `effects`: `Config.Warnings []string`, `RepoConfigFile = ".claude/agentik/config.yaml"`. `main`: `aliasNote(argv0, verb string, tty bool) string`. `plugin/hooks/run`: `$RELAY_BIN`, else `agentik-relay`, else `relay`. Makefile: `BINDIR ?= $(HOME)/.local/bin`; `make -n install BINDIR=D` prints, in order, `mkdir -p D`, `go build -o D/agentik-relay ./cmd/agentik-relay`, `ln -sf agentik-relay D/relay`, `install -m 0755 scripts/agentik D/agentik`, then one line relinking `$(go env GOPATH)/bin/relay` to `D/agentik-relay` only when it exists; `build`, `doctor`, `e2e` name `./cmd/agentik-relay`.

Tests `TestAgentikR1_*`. hooks: `1_RowDeniesEverySpelling` (A6's five, `agentik-session start`, `go build ./cmd/agentik-relay`; `agentik relay status` passes), `1_HelperDeniesEverySpelling`, `3_StateRootNewElseOld`. effects: `3_StateDirNewElseOld`, `3_LayersNewElseOld`, `3_RelaySectionWins`. night: `3_PidDirNewElseOld`, `3_HooksAndRelayAgree` (SoftPath under StateDir), `4_ResumeLines`, `4_OneResumeLine` (no non-test Go file outside `internal/relay` holds `relay resume "`). main: `4_StopLineReadsBoth`, `4_OldNameNote`, `5_InstallTarget`. packet: `5_PluginIsRelay` (name `relay`, no repo marketplace, `Bash(agentik-relay *)`, the shim's order). hygiene: `2_VendoredPinned` (three `OK`), `6_RenamedZero`. Red on the pin: `1_RowDeniesEverySpelling`.

## 4. Findings that name this row

§2.4: the guards match `^claude-session` (`guards.go:815`) and the basename `relay` or `cmd/relay` (`:66,108`); a new spelling passes both. Not this row's: `rules-orchestrator.txt` asks a row for the session's `charter open`, which the session guard denies in a row (`:892`); rename it, file it in NICE_TO_HAVE.md.

## 5. Forbidden reads

Transcripts; the CLI's project records; `~/.local/state`; the hygiene private list (never print or copy it); any other repo but the commons files named above.

## 6. Report (capped)

At most 200 words. Table: each acceptance count, `make doclint`'s and `make validate`'s exits, `make hygiene`'s last line. Bullets: the red tests on the pin; existing tests changed, by count; each `.hygiene-allow` row; each fork beyond §2, one sentence; filings. Last line: branch, head sha.
