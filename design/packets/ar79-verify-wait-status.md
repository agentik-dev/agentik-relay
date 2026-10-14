# Packet AR79: status names a verify waiting for its root's gate lock

| item | value |
|---|---|
| from | `design/verify-cut-r1.md` §3's `status.json` bullet and §5 packet 2 (bedc7c2, refuted PASS, §7); `design/packets/ar78-gate-lock.md` (rulings 1 and 5: `GateLock`'s two `waiting` calls, `launch`'s `Waiting`); `NICE_TO_HAVE.md:369` (ar59-2); `internal/night/status.go` and `night.go` at AR78's merge |
| pin | agent-relay `main` at AR78's merge commit plus this packet's own commit; a linked worktree, branch `ar79-verify-wait-status`. Never before AR78 lands: it reads AR78's `GateWait` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/night ./cmd/agentik-relay`, `gofmt -l internal/night`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/status.go` (`RowStatus`, `PrintStatus`, a `statusNow` var, and their comments only), `internal/night/night.go` (`launch`'s `Waiting` only), `internal/night/testdata/json-keys.golden` (the one line `status rows.[]wait`), `internal/night/ar79_test.go` (new), `internal/night/ar79_internal_test.go` (new), `design/relay-r1.md` (§9's `status` row and §11's test list only), `design/verify-cut-r1.md` (its Status line only), `NICE_TO_HAVE.md` (a new section for what the build leaves), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR79_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR79_'` reads 2; `go test ./cmd/agentik-relay -run '^TestJSONShapesFrozen$' -count=1 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./internal/night -run '^Test(AR47Fix_\|AR51_\|AR77_\|AR78_\|AR79_\|Status)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 |
| budget | 50 turns, 150k context, `claude-opus-5-5`; one act, one commit |
| verify | `make check` |

## 1. What the source says (quoted)

verify-cut-r1 §3: "`status.json`: each row gains `wait`, `verify behind <row> since <hh:mm:ss>Z` from `waiting`'s mark until `GateLock` returns, else `""`; the text line ends `verify waits behind <row> <m>m`. The row's `state` stays `running`, the night's `running` or `draining`. The key re-freezes `internal/night/testdata/json-keys.golden`." §5 packet 2: "§3's `wait` key and text line; a row blocked in `GateLock` reads `wait` from `waiting`'s mark, any other `""`. Its may-touch names `json-keys.golden` (§3): only its absence barred AR59 (`NICE_TO_HAVE.md:369`). After AR78." §7, note 7: "`waiting` marks a wait before it blocks".

AR78's packet, ruling 1: `GateLock` calls `waiting(holder, since)` before it blocks and, when that wait ends, held or killed, `waiting("", since)` once; ruling 5: `launch` fills `GateWait.Waiting`, which puts the `verify-wait` line on a non-empty `behind`.

The code at 7450c05: `RowStatus` `internal/night/status.go:47-66` (`Cause` its last field), `PrintStatus` `:272`, its row cells `:294-306`, `table` `:366` (a row may carry more cells than the header); status is written each loop event and each poll (`night.go:696-716`, `:1725`); the golden's status rows `testdata/json-keys.golden:165-183`, `rows.[]unread` then `rows.[]worktree`.

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar79.md` in doclint's five-part shape, opened at the act, closed before the commit, no home path. The act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is.

1. **One key.** `RowStatus` gains `Wait string` with `json:"wait"` after `Cause`; the golden gains `status rows.[]wait` between `unread` and `worktree`, its only change. `Check.Wait` and `Cut` stay `json:"-"` (§3: "as `Cut` is"): ar59-2's `cut` and `verify_seconds` stay filed, not this row's.
2. **The mark.** `launch`'s `Waiting`, under `lg.mu`: on a non-empty `behind`, after its `verify-wait` line, `lg.rows[id].Wait` is `verify behind <behind> since <since in UTC, 15:04:05>Z`; on `""` it is `""`. Nothing else writes it, so every other row, and the row before and after its wait, reads `""`. No `state`, `reason` or `next` reads it.
3. **The text line.** `PrintStatus` gives a row whose `wait` is set one more cell after `merge`, under no header: `verify waits behind <row> <m>m`, `<row>` and the clock read back from the value, `<m>` the whole minutes from that clock to `statusNow()` in UTC (a clock later than now is the day before: add 24 h). `statusNow` is a package var, `time.Now`, which only the internal test sets. A value that does not parse prints as it is.
4. **Tests.** `TestAR79_1_StatusNamesTheWait` (`night_test`): `nightWorld` r1 (verify `block 1500`, AR78's `blockShell`) and r2 (no verify line), `--parallel 2`, `Poll` 20 ms, `h0` holding r1's root (AR78 ruling 6): while r1's `verify-wait` stands, status.json's r1 reads `wait` matching `^verify behind h0 since \d\d:\d\d:\d\dZ$` within 2 s of the line, `state` `running`, the night `running`, r2's `wait` `""` throughout; `stop` written: the night `draining`, r1's `wait` unchanged; `h0` freed: while r1's line runs its `wait` reads `""`; at the end every `wait` is `""`, both green. `TestAR79_2_TextLine` (internal, `statusNow` at 00:02:00Z): `PrintStatus` over r1 `running` with `wait` `verify behind h0 since 23:59:00Z` and r2 `green` with none: r1's line ends `verify waits behind h0 3m`, r2's ends in its `merge` cell, the header keeps its ten cells; `json.Marshal(RowStatus{})` holds `"wait":""`.
5. **The record.** relay-r1 §9's `status` row: a row whose verify waits for its root's gate lock reads `wait` (AR79); §11 lists the two tests. verify-cut-r1's Status line gains "AR79 built §5 packet 2".

## 3. Interfaces (signatures, not prose)

`RowStatus.Wait string` (`json:"wait"`); `var statusNow = time.Now`. `PrintStatus(w io.Writer, s Status)` and `buildStatus` keep their signatures; AR78's `GateLock`, `GateWait` and `verify-wait` line are unchanged. One `--json` key: `status rows.[]wait`.

## 4. Findings that name this row

`NICE_TO_HAVE.md:369` (ar59-2): the golden's absence from AR59's may-touch kept `cut` and `verify_seconds` out of `--json`; this row may touch it and adds `wait` alone (ruling 1). verify-cut-r1 §3's `status.json` bullet.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo; `design/relay-r1.md` by sections only.

## 6. Report (capped)

At most 120 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: TestAR79_1's red line on the pin; the golden's diff; anything filed. Last line: branch, head sha.
