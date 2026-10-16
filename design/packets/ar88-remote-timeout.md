# Packet AR88: a gate's ls-remote timeout retries, then names `remote-timeout`

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`: "The observer, check-in 109 (2026-10-13), S3"; `design/relay-r1.md` §6; the coordinator's rulings (2026-10-16) |
| pin | `main` at 6fd429b plus this packet's own commit; a linked worktree, branch `ar88-remote-timeout` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` or `agentik relay` verb against the account's state: a night may be live. Never run `agentik-session`. Hygiene's `lan` rule matches the Go name of the local zone: never spell it. |
| may touch | `internal/effects/git.go` (`ErrLsRemoteTimeout`, `LsRemote`'s timeout error), `internal/relay/types.go` (two reasons), `internal/relay/gate.go` (`Gate.Remote`, `Decide`), `internal/gate/gate.go` (`LsRemoteTries`, `LsRemoteBackoff`, `lsRemote`, `remotesSame`'s read, the gate's three `ls-remote` stops, `Cause`), `internal/night/run.go` (`RunRow`'s unrun reasons), `internal/gate/ar88_test.go` (new), `internal/night/ar88_test.go` (new), `design/relay-r1.md` (one paragraph), `RUNBOOK.md` (only if it lists the unrun reasons) |
| acceptance | `go test ./internal/gate -run '^TestAR88_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR88_'` reads 3; `go test ./internal/night -run '^TestAR88_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR88_'` reads 1; `go test ./internal/gate -run 'TestAR17A\|TestAR20_3\|TestAR22_4\|TestAR31_2\|TestAR62' -count=1 -timeout 600s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0; `go test ./internal/night -run 'TestAR17A\|TestAR62' -count=1 -timeout 300s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0; `go test ./internal/effects -run 'TestAR20_3' -count=1 -timeout 300s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0; `go test ./internal/relay -run 'TestDecide' -count=1 -timeout 120s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0 |
| budget | 45 turns, 180k context, `claude-opus-5-5`; three acts (this packet, the fix, the record), one commit each |
| verify | `make check` |

## 1. What the source says (quoted)

NICE, check-in 109: "a transient `git ls-remote` timeout during the gate makes a finished row `unrun` with an empty reason, and nothing retries it. Night 2026-10-13-2, an example-app row, 15:01:59Z: `{"detail":"ls-remote: git ls-remote https://github.com/owner/example-app.git: timed out after 30s","event":"gate","reason":"","result":"unrun"}`; `relay status` and `relay report` show `cause: ""`, so the timeout is visible only in the ledger. … Name a reason (for example `remote-timeout`) so status and report carry it, and retry the read once or twice before giving up."

On the pin: `gather` stops `ls-remote` at three sites (`gate.go:338`, `:352`, `:496`), each through `remotesSame`, which calls `Git.LsRemote` once per remote. `effects.LsRemote` bounds one read at `LsRemoteTimeout` (30 s) and returns a plain `fmt.Errorf`. `relay.Decide` returns `Unrun, ""` for an ungathered gate; `RunRow` (`run.go:899`) then sets the row's reason `gate-unrun` unless the gate named one of three; `Cause` reads `Red`, which names nothing for an ungathered gate, so `cause` is `""`.

## 2. Rulings

1. **The retry.** One remote's read is up to `LsRemoteTries` (3) calls: a call that times out (`errors.Is(err, effects.ErrLsRemoteTimeout)`) sleeps `LsRemoteBackoff` (5 s) and reads again; another error is not retried (a missing repo or a refused key answers the same twice). Both are package variables a test sets. A read that succeeds on a retry is the read: the row proceeds as if the first had. The merge's read (`remotesSame`, `gate.go:1682`) gets the retry too; its `aborted` line and error keep their shape.
2. **The reasons.** Two new unrun reasons, packet failures absent from `StopReasons`: `remote-timeout`, the last try timed out; `remote-error`, any other ls-remote failure. Each gate stop at `ls-remote` sets `Gate.Remote` to one of them; `Decide` returns `Unrun, g.Remote` after `ToolMoved`. The gate's end line reads `reason: remote-timeout`; its `detail` keeps `ls-remote: git ls-remote <url>: timed out after 30s`.
3. **The row.** `RunRow`'s unrun case carries `RemoteTimeout` and `RemoteError` as the row's reason, as it carries `git-state-moved`: the row is red with it, never `gate-unrun`.
4. **The cause.** `Cause` of a gate with `Remote` set begins `ls-remote: <reason>` (`ls-remote: remote-timeout`), so the end line, the `sitting` line's `gate.cause`, `status` and `report` carry it. `Red` and the `sitting` line's `gate.red` do not change (the gate gathered nothing).
5. **Tests**, `internal/gate/ar88_test.go` on `newWorld` (its bare `origin`), staged, then `w.fx.Git` a wrapper of `OSGit` whose `LsRemote` fails as told, `LsRemoteBackoff` a millisecond:
   - `TestAR88_1_TimeoutOnceProceeds`: the first gate read times out, the rest pass: not unrun, no error, the end line's reason `""`, and the wrapper counted the extra read.
   - `TestAR88_2_TimeoutEveryRead`: every read times out: `Unrun, remote-timeout`, the error `ls-remote: …timed out…`, three reads, the end line's reason `remote-timeout` and cause `ls-remote: remote-timeout`.
   - `TestAR88_3_OtherErrorAtOnce`: a plain error: `Unrun, remote-error`, one read, no backoff.
   - `internal/night/ar88_test.go`, `TestAR88_4_StatusAndReport`: one row over `nightWorld` with an `origin`, every gate read timing out: the row red `remote-timeout`; status.json's row and the report's stopped row carry the cause `ls-remote: remote-timeout`.

## 3. Interfaces (signatures, not prose)

`effects`: `var ErrLsRemoteTimeout = errors.New("timed out")`, wrapped by `LsRemote`'s timeout error, its text unchanged. `relay`: `RemoteTimeout StopReason = "remote-timeout"`, `RemoteError StopReason = "remote-error"`; `Gate.Remote StopReason` (`json:"-"`). `gate`: `var LsRemoteTries = 3`, `var LsRemoteBackoff = 5 * time.Second`; unexported `lsRemote(git effects.Git, dir, url string) (map[string]string, error)`. No ledger key, status field or golden changes; the end line's `reason` gains two values.

## 4. Findings that name this row

NICE's check-in 109 entry (the coordinator marks it; another branch edits NICE). A row whose remote stays down for the three reads is still unrun, now named: the retry covers a transient stall, not an outage.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory and its ledgers; other repos.

## 6. Report (capped)

At most 120 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits. Bullets: the tests red on the pin, and how; any ruling beyond §2.
