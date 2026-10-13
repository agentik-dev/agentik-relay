# Packet AR17C: the suite by any spelling, the killed plan, the escaped daemon, the two lock races (ADV10-4, -5, -12, -13, -14, -21, -23)

| item | value |
|---|---|
| from | `design/review-adv10.md` whole; example-mac's `history/relay/2026-09-23-defects.md` D2 §4, §6, §9; `design/relay-r1.md` §9, §11, §15, §16; `design/tla/`; the code the review cites. Never a ledger but a fixture's |
| pin | agent-relay `main` after `ar17b-guards-r10` merged (cut off `main`, no `base`); worktree `.claude/worktrees/ar17c-bounds-r10` |
| wall | this repo only. No push, `--amend`, rebase, device or network; every test path under `t.TempDir()`, never the real queue, ledger or state root; a hand `relay plan` only with `--rows`, `RELAY_MEASURE_DEPTH=1` and a temp `XDG_STATE_HOME`; `go test ./...` only inside `make check` |
| may touch | `internal/packet/`, `internal/night/`, `internal/effects/`, `internal/stream/`, `cmd/relay/`, `scripts/bounded`, `design/relay-r1.md`, `design/tla/Night.tla`, `design/tla/Night.cfg`, `history/sittings/*.md`, `history/sessions/NEXT` (removed if present). Nothing else |
| acceptance | `for p in packet night effects stream; do go test ./internal/$p -run TestAR17C_ -count=1 >/dev/null 2>&1 \|\| exit 1; done; echo ok` reads `ok`; `go test ./cmd/relay -run TestAR17C_ -count=1 2>&1 \| cut -c1-2 \| tail -1` reads `ok`; `grep -rhc '^func TestAR17C_' internal cmd \| paste -sd+ - \| bc` reads 9 or more; `grep -c 'TestAR17C_' design/relay-r1.md` reads 6 or more; `test -e history/sessions/NEXT; echo $?` reads 1 |
| budget | 80 turns, 130k context, `claude-opus-5-5`; one commit per act (0 to 4) plus the charter's; `make check` is the verify line, never acceptance; checkpoint at 80%; the charter is the first free `history/sittings/2026-09-23-<n>-ar17c.md`, closed before the last commit |
| verify | `make check` |

## 1. What the source says (quoted)

review-adv10: "The one fix that matters most: ADV10-4 ... close by target name and command-word shape"; "three doors remain: another spelling of the suite (4), `go run` (6), a killed plan's orphan (5)". Its tests 2 and 3: "`TestADV10_UnboundedSpellings`" and "`TestADV10_SignalMidPlan`: SIGTERM to `relay plan --rows r` mid-baseline; no descendant alive, `groups.json` records the line."

## 2. Rulings

Decide inside the design, record each choice in the charter, at most ONE fork. One commit per act; each act's test reproduces the finding FIRST (red on `main`), then the fix. `scripts/bounded` is this repo's own (not a conventions §6 pinned copy); `scripts/doclint` and `scripts/hygiene` are pinned and stay untouched here.

0. **The suite by any spelling (ADV10-4).** `packet.Unbounded` refuses, per segment after `hooks.Normalise`: `make` or `gmake` with any target among `check`, `check-*`, `test`, `vet`, `e2e`, `tla`, or with no target at all (the default goal); a command word that is not a literal word (`$m`, `$(...)`, a backtick); a shell fed from stdin, a here-string or a pipe (`echo 'make check' \| sh`). Allowed: named narrow targets (`make fmt`, `make hygiene`, `make doclint`). `TestAR17C_0_UnboundedSpellings` holds every spelling the review ran; `TestAR17C_0_NarrowTargetsAllowed`; the packets in `design/packets/` from `ar10-*` on still parse (`TestAR17C_0_LivePacketsParse`, reading copies under `t.TempDir()`).
1. **Trap before any measurement (ADV10-5).** `main` installs the INT, TERM, HUP trap and the group registry (`groups.json`) before `loadPlan`, for `run`, `plan`, `gate` and `resume`; every measured line registers its group before it starts. A signal reaps every registered group (TERM, `KillGrace`, KILL), writes what it can, exits 1. The review's test 3, as `TestAR17C_1_SignalMidPlan` (the built binary, a fixture repo whose check line is `sleep 45`); `TestAR17C_1_SignalMidGate`. §16: the `measuring` half of `NoOrphanAfterExit` maps to them.
2. **The escaped daemon (ADV10-12; D2 §9).** A measured line and a child each get their own session (`Setsid`); the sweep after exit kills every process whose session id is that session's, from a snapshot taken at start and after the exit (`effects.Procs.Session(pid)`), and counts it. A line with a survivor is `unrun: orphans`, never green. `TestAR17C_2_SetsidDaemonSwept` (the review's perl spelling). The model: an `escaped` process kind the sweep reaps, `NoOrphanAfterExit` over it.
3. **The two lock races (ADV10-13, -14).** The stage lock is taken by `flock` (`LOCK_EX\|LOCK_NB`) on the lock file held open for the stage's life; the read, remove, create takeover goes. `scripts/bounded` never takes a lock whose pid is alive, whatever its age; a dead pid's lock is taken over once. `TestAR17C_3_StageLockOneHolder` (six concurrent takers, one holder, 40 runs), `TestAR17C_3_BoundedKeepsLiveLock` (a shell line under a minute through `scripts/bounded`).
4. **The lows and the design (ADV10-21, -23).** `LiveClaude`'s timeout kills the group (`stage.go:69`): `TestAR17C_4_ProbeLeavesNoOrphan`. `stream` reads lines through a cap (1 MiB): a longer line is `non_json` and the stream goes on: `TestAR17C_4_LongLineBounded`. relay-r1 §15 lists the refused shapes once; §9 the trap; §11's table and §16's map name every `TestAR17C_`; `make tla` reads `tla: ok` once before the closing commit (reported).

## 3. Interfaces (signatures, not prose)

- `packet.Unbounded(line string) string` (unchanged signature); `effects.Procs.Session(pid int) (int, error)`; `night.Trap(ctx context.Context, reg *effects.Groups) (context.Context, func())` used by every measuring verb; `stream.MaxLine = 1 << 20`.

## 4. Findings that name this row

Baseline on `main` after ar17b merged: ADV10-4 at `packet.go:179-184`; ADV10-5 at `main.go:889` and `night.go:291`; ADV10-12 by the review's `perl` line; ADV10-13 at `stage.go:161-173`; ADV10-14 by `STALE=900`; ADV10-21 at `stage.go:69`; ADV10-23 at `stream.go:202`.

## 5. Forbidden reads

Every other repo but the one defects file named in `from`; `~/.claude/projects/`; the real state root; any ledger but a fixture's; the packets before `ar13-*`; `.claude/worktrees/` but your own.

## 6. Report (capped)

The table: each acceptance line, its exit and last line. Bullets: per act the reproducing test's red line on `main`; `make tla`'s last line; what was left undone; the one open fork. Last line: the branch and its head sha.
