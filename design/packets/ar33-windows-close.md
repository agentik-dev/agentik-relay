# Packet AR33: a night's windows end with the night

| item | value |
|---|---|
| from | `design/relay-r1.md` §9 (the CLI, `status` and `attach`) and D16 (the night's windows); the observation below |
| pin | agent-relay `main` at `db78044` plus this packet's own commit; worktree `.claude/worktrees/ar33-windows-close`, branch `ar33-windows-close` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, no live `claude`, no test that sleeps more than 2 s in all. `go test ./...` only inside `make check`, in the foreground. No `relay run`, no `relay plan`. No `make tla` or `make e2e`. A real-tmux test uses its own socket in its temp dir, never the account's |
| may touch | `internal/night/`, `internal/effects/`, `cmd/relay/`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR33_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR33_'` reads 3; `go test ./internal/effects -run '^TestAR33_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR33_'` reads 2; `go test ./cmd/relay -run '^TestAR33_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR33_'` reads 2 |
| budget | 120 turns, 150k context, `claude-opus-5-5`; one commit per act (1 and 2); `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`internal/night/windows.go:10-13`: "Windows is the night's viewers (§9, D16): under tmux a session relay-<night> whose first window loops `relay status` and a window per row tailing the row's log". Nothing in the source closes that session. What was seen:

On the owner's Mac on 2026-09-29 at 22:04, the relay's tmux socket held ten sessions `relay-<night>`, one per night that day. Nine of those nights had stopped hours earlier (`stopped:queue-empty`, `count-reached`, `owner-kill`, `budget`). Each session's first window still ran `StatusArgv`, `while :; do clear; relay status; sleep 5; done`, and each `relay status` process took about 110% of a core for 0.8 s. Together they took about 1.8 cores under a load of 15, while the running night's baseline measure was timing out under that load. Because `StatusArgv` passes no `--night`, all nine old windows showed the latest night, not their own. The relay opens a session per night (`windows.go:37-63`) and never closes one: `effects.Tmux` has no kill verb (`tmux.go:13-20`).

## 2. Rulings

Standing rules. Work in your worktree on branch `ar33-windows-close` off `main`. The charter is `history/sittings/2026-09-29-5-ar33.md` (the `-4` is ar32c's on its own branch), in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit. `DECISIONS.md` stays as it is; a fork this packet does not settle goes in the charter with the choice made.

1. **The status window watches its own night and stops with it.** `relay status --night D --watch N` redraws the dashboard in-process every N seconds (N at least 1; clear with ANSI home and erase, not by spawning `clear`), with no new process per tick. It prints the dashboard once more and stops redrawing when the night's state is `stopped:<reason>` (a `draining` night is still live), or when the night's own relay is gone: not live, or live under another start time. The night's own relay is the pid and start time the night records while it runs. `groups.json` holds both, and `status.json` gains `pid` and `started` if it carries neither; use `PidStarted` for liveness. `--hold` with `--watch` then waits for a signal, using no CPU, so the pane keeps the final table; without `--hold` the verb exits 0. `--watch` without `--night` watches the latest night, as `status` reads it. `StatusArgv` becomes `StatusArgv(night string) []string`, whose argv is `<relay> status --night <night> --watch 5 --hold` with no shell. `<relay>` is the running binary's path (`os.Executable`), falling back to `relay`. `Open` passes the night. Tests: `TestAR33_1_WatchStopsWhenNightEnds` and `TestAR33_1_WatchStopsWhenRelayGone` (cmd/relay; the sleep and the clock are injectable, so neither test sleeps for real); `TestAR33_1_StatusWindowNamesItsNight` (night; the argv carries the night, `--watch` and `--hold`, and no `sh`).
2. **A night's open closes the old nights' sessions.** There is one relay per account (§8), so when a night opens, no other night on this account is live. `Open`, under `auto` or `required` with tmux present, lists the sessions on the relay's socket and kills each one named `relay-` plus anything other than this night's name, before it starts its own. Under `never` it touches nothing. A failed list or kill is not an error: it prints one line, `relay: tmux: <err>`, and the night goes on. `effects.Tmux` gains `Sessions() ([]string, error)`, which is `list-sessions -F '#{session_name}'`, where "no server running" or "no sessions" reads as an empty list, not an error. It also gains `Kill(session string) error`, which is `kill-session -t =<session>`: the `=` makes the target exact, so that killing `relay-X-1` can never match `relay-X-10` by prefix. `Alive` and the attach argv take the same exact target (`has-session -t =<session>`, `attach -t =<session>[:<window>]`), since has-session matches a prefix too. `FakeTmux` gets a settable session list and records `Sessions` and `Kill`. The night writes one ledger line, `windows-closed`, naming the sessions it killed, and none when it killed none. Tests: `TestAR33_2_OpenClosesOldNights` (night; the list `relay-a`, `relay-b`, `other`, and this night's name: kills exactly `relay-a` and `relay-b`); `TestAR33_2_NeverTouchesNothing` (night); `TestAR33_2_KillIsExact` (effects, real tmux on a socket in the temp dir, skipped without tmux: sessions `relay-t-1` and `relay-t-10`, kill `relay-t-1`, then kill `relay-t-1` again; `Alive("relay-t-1")` is false and `relay-t-10` is still alive); `TestAR33_2_SessionsNoServer` (effects, real tmux, skipped without tmux: a socket with no server lists empty, no error).

After the acts: `design/relay-r1.md` §9's `status` row gains `[--watch N [--hold]]` with one sentence, and the windows paragraph (search `D16`) gains one sentence each on the watch that ends with its night and on the sweep at open. Existing tests that compare `StatusArgv` move to `StatusArgv(night)`.

## 3. Interfaces (signatures, not prose)

`night.StatusArgv(night string) []string`. `effects.Tmux` gains `Sessions() ([]string, error)` and `Kill(session string) error`; `OSTmux` and `FakeTmux` implement both (`FakeTmux.List []string`). `statusVerb` gains `--watch` (a value flag) and `--hold` (a bool flag). The watch loop takes an injectable sleep and an injectable liveness function, so tests need no real time.

## 4. Findings that name this row

On `main` at `db78044`: `internal/night/windows.go:23-24` (`StatusArgv`), `:37-63` (`Open`); `internal/effects/tmux.go:13-20` (the interface), `:103-109` (`NewSession`, `NewWindow`), `:111-127` (`Alive`, whose has-session also takes a prefix target); `internal/effects/fake.go` (`FakeTmux`); `internal/night/night.go:437` (the `Open` call), `:400-410` (`groups.json` with pid and started); `internal/night/pidfile.go:108-110` (`PidStarted`); `cmd/relay/main.go:1302-1328` (`stateAndNight`, `statusVerb`); `internal/night/windows_test.go:34`, `:74` (the argv compared).

## 5. Forbidden reads

Other repos; the real state root, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours. Never touch the account's own tmux socket or its sessions.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: per act the red test on `main`; what was left. Last line: branch, head sha.
