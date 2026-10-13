# Packet AR39: the latest night is a dated one

| item | value |
|---|---|
| from | `design/relay-r1.md` §9 (`status`, `stop` / `kill`, `report`: "the latest night" when `--night` is not given); the observation below |
| pin | agent-relay `main` at 3ed3b03 plus this packet's own commit; worktree `.claude/worktrees/ar39-latest-skips-hand`, branch `ar39-latest-skips-hand` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state |
| may touch | `internal/night/`, `cmd/relay/`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR39_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR39_'` reads 2; `go test ./cmd/relay -run '^TestAR39_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR39_'` reads 1 |
| budget | 60 turns, 80k context, `claude-opus-5-5`; one commit for the act; `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`internal/night/status.go:309-340`: `Nights` lists every name under `nights/`, sorted by the text before the last `-` and then by the number after it; `Latest` is the last of them. `cmd/relay/main.go:1309-1317`: `stateAndNight` returns `--night` when given, else `night.Latest`. `cmd/relay/main.go:800-801`: `relay gate` writes its state under the night name `hand`.

What was seen on 2026-09-30. After a hand-run `relay gate`, `nights/` held `2026-09-29-1` to `2026-09-29-11` and `hand`. `Nights`' key for `hand` is `("hand", 0)`, and `"hand"` sorts after every `"2026-…"`, so `Latest` is `hand`. `relay status` with no `--night` printed `night hand`, and `relay stop` or `relay kill` with no `--night` would write the control file into `nights/hand/`, not into the running night's dir. Readers have worked around it with `--night <id>`.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar39-latest-skips-hand` off `main`. The charter is `history/sittings/2026-09-30-6-ar39.md` in doclint's five-part shape, opened at the act, closed before the last commit, no home path. The act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is.

1. **Nights lists dated nights only.** `Nights` returns only the names that match `<YYYY-MM-DD>-<n>` (`^\d{4}-\d{2}-\d{2}-\d+$`), sorted as today. `hand` and any other name are left out, so `Latest` is the newest dated night, or `""` when there is none. `--night hand` still reads the gate's dir, since `stateAndNight` passes an explicit `--night` through unchanged. `NightName` and `Reap` keep their own listing: `Reap` must still see `hand/groups.json`. Tests: `TestAR39_1_LatestSkipsHand` (night) makes `nights/2026-09-29-9`, `nights/2026-09-29-11` and `nights/hand`, and expects `Latest` to be `2026-09-29-11`. `TestAR39_1_NightsDatedOnly` (night) makes `hand`, `notes` and `2026-09-30-x` beside `2026-09-30-2`, and expects `Nights` to be `[2026-09-30-2]`, and `Latest` to be `""` when only `hand` exists. `TestAR39_1_StatusDefaultSkipsHand` (cmd/relay) runs `status` with no `--night` over a state holding a dated night's status.json and a `hand` dir. The header names the dated night.

After the act, §9 gains one sentence: "the latest night" is the newest `<date>-<n>` dir, and the gate's `hand` dir is never the latest.

## 3. Interfaces (signatures, not prose)

No signature changes. `Nights(fs effects.FS, state string) []string` filters before it sorts.

## 4. Findings that name this row

`internal/night/status.go:309-340` (`Nights`, `Latest`); `cmd/relay/main.go:1309-1317` (`stateAndNight`); `internal/night/plan.go:185-198` (`NightName`, unchanged); `internal/night/groups.go:60` (`Reap`, unchanged); `cmd/relay/main.go:800` (the gate's `hand`).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: the red test on `main`; what was left. Last line: branch, head sha.
