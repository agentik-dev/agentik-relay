# Packet AR36: no other lock writes the account's relay.pid

| item | value |
|---|---|
| from | `design/relay-r1.md` §8 ("one relay per account") and AR22 act 2 (the port lock, the pid file, `lock-moved`); the observation below |
| pin | agent-relay `main` at `7bd029d` plus this packet's own commit; worktree `.claude/worktrees/ar36-pid-file-guard`, branch `ar36-pid-file-guard` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs; every pid file in a `t.TempDir()`. Never build a `relay` binary by hand, and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir, which a live relay may hold. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate` or `resume` |
| may touch | `internal/night/pidfile.go`, `internal/night/*_test.go`, `cmd/relay/main.go`, `cmd/relay/*_test.go`, `plugin/agents/builder.md`, `plugin/agents/adversary.md`, `design/relay-r1.md`, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR36_' -count=1 -v 2>&1 \| grep -c '^--- PASS: TestAR36_'` reads 3; `grep -l 'PidHomeLink' plugin/agents/builder.md plugin/agents/adversary.md \| grep -c agents` reads 2 |
| budget | 100 turns, 120k context, `claude-opus-5-5`; one commit per act (1 to 3); `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`pidfile.go`: "the lock is the port, never the file." `PidFile`: "then <state>/relay.pid, written by a rename, whose one line `<pid> <started>` names the holder … release removes the pid file if it is still the one written". `LockPortLink`: "set at link time only (-ldflags -X), for a test's built binary: its account lock's port in place of the uid's."

What was seen on 2026-09-30 between 06:56Z and 06:58Z. A review agent built `relay` with `-ldflags "-X …/night.LockPortLink=29871"` and no `PidHomeLink`, and ran `relay plan` four times against a scratch repo. Its HOME and XDG_STATE_HOME were scratch, but `PidDir` reads the home from the user database, so it was the account's real `~/.local/state/agent-relay`. Port 29871 was free, so each run took "the lock" and renamed its own line over the live relay's `relay.pid` (`:167`). On release it found the file was the one it wrote, and removed it (`:184-185`). The live night (a plain release binary, pid 75368) read the file as gone at its next poll, and stopped `lock-moved`: no new sitting started, and nine rows stayed ready overnight.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar36-pid-file-guard` off `main`. The charter is `history/sittings/2026-09-30-3-ar36.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is.

1. **A live holder's file is never replaced.** Before `PidFile` writes, it reads the file at its path. If that file names a live holder (`liveHolder`, pid and start time) other than this process, `PidFile` releases the port it just took and returns an error naming that holder and its path: `relay.pid names live relay <pid>, which holds another lock; this process is on port <p>`. A file that names a dead or reused pid is replaced as today, since that is crash recovery. Tests: `TestAR36_1_LiveHolderFileIsNotReplaced` writes into a temp dir a line naming this test's parent, `os.Getppid()`, with its real start time from `PidStarted`, then calls `PidFile(dir)` on a free port. It expects an error, the file's inode and text unchanged, and the port free again. `TestAR36_1_DeadHolderFileIsReplaced` does the same with a line naming a pid that is not live, and expects the lock taken and the file rewritten.
2. **A linked port needs a linked home.** A binary linked with `LockPortLink` and without `PidHomeLink` refuses to resolve its pid dir. `PidDir` returns `a linked lock port (LockPortLink) needs a linked pid home (PidHomeLink): refusing the account's real pid dir`, so `holdPid` exits 2 before it takes anything. Test: `TestAR36_2_LinkedPortNeedsLinkedHome` sets the package vars in-process and restores them. It never builds or runs a binary.
3. **The rule in the agents' own words.** `plugin/agents/builder.md` and `plugin/agents/adversary.md` each gain one line under their standing rules. The line says: never build `relay` by hand, and never run a hand-built one. A probe goes through `make`'s test harness, or links both `LockPortLink` and `PidHomeLink` at a temp dir, because the account's pid dir is read from the user database, not HOME. A relay night may be live on this account.

After the acts, `design/relay-r1.md`'s AR22 paragraph (search `lock-moved`) gains one sentence each for rulings 1 and 2. A `NICE_TO_HAVE.md` line asks whether a missing advisory file should stop a night whose port is still held, or should be rewritten by its holder. That is not decided here.

## 3. Interfaces (signatures, not prose)

No signature changes. `PidFile(state string) (func(), error)` gains the refusal. `PidDir() (string, error)` gains the linked-port check. The error texts are ruled above.

## 4. Findings that name this row

On `main` at `7bd029d`: `internal/night/pidfile.go:18-26` (`PidFileName`, `LockPortLink`), `:40-56` (`LockPort`), `:59-90` (`PidHomeLink`, `UserHome`, `PidDir`), `:138-189` (`PidFile`, its write at `:167`, its release at `:184-185`), `:304-325` (`liveHolder`), `:328-345` (`LockMoved`); `cmd/relay/main.go:1480-1500` (`holdPid`); `cmd/relay/main_test.go:1105-1115` (the linked test binaries set both links).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: per act the red test on `main`; what was left. Last line: branch, head sha.
