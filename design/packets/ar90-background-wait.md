# Packet AR90: a child that waits for background work, read on 2.1.302; the bare `Bash` kept, its measurement recorded

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md` (§4's items); `design/relay-r1.md` §5, §7, §15; the coordinator's probes of 2026-10-16 (§1); the code at 15a2920 |
| pin | `main` at 15a2920 plus this packet's own commit, which also adds `internal/effects/testdata/fakeclaude/fixtures/bg-wait.jsonl` (41 lines, the probe's stream); a linked worktree, branch `ar90-background-wait` |
| wall | AR83's: this repo only; no push, `--amend`, rebase, device, ssh or network; no `relay` binary by hand or outside `make`'s harness; no `relay` or `agentik relay` verb on the account's state; never `claude`. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet` and `gofmt -l` of the three packages, `make doclint`, `make hygiene`, one at a time at `nice -n 10`. The fixture is read, never edited. Never spell the Go name of the local zone. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/effects/config.go` (`DefaultAllowedTools`'s comment only), `internal/effects/ar90_test.go` (new), `internal/stream/stream.go` (`Tally.Results`, `Add`'s `result` case, the `Tally` comment only), `internal/stream/ar90_test.go` (new), `internal/night/run.go` (the `exit` line's `results` only), `internal/night/ar90_test.go` (new), `design/relay-r1.md` (§5's "One turn count", §7's spawn block and permissions paragraph, §15's background-work bullet, §11's test list only), `NICE_TO_HAVE.md` (marks on §4's items; one new section), `history/sittings/*.md`, `history/sessions/NEXT`. Nothing else |
| acceptance | `go test ./internal/effects -run '^TestAR90_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR90_'` reads 1; `go test ./internal/stream -run '^TestAR90_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR90_'` reads 2; `go test ./internal/night -run '^TestAR90_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR90_'` reads 2; `go test ./internal/stream -run '^Test(ParseFixtures\|TallyLine\|AR83_\|AR86_)' -count=1 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./internal/night -run '^Test(Plan\|WallClockKill\|AR10_6_\|AR10_8_\|AR32A_1_\|AR51_\|AR83_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1; `grep -c -E '"subtype":"init"\|"type":"result"' internal/effects/testdata/fakeclaude/fixtures/bg-wait.jsonl` reads 4 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; four acts, one commit each; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says (quoted)

NICE item 3: auto mode "drops a bare `Bash` from `--allowedTools`, which `effects.DefaultAllowedTools` passes … read `permission_denials`, and decide whether the spawn line should name Bash rules or another mode." Item 1: "Check … how long a child's exit trails its `result` event, whether any rule reads that `result` as the end of the sitting, what the sweep finds, and record the CLI version the rules are tuned on." The S4s: "Drop the entry"; the stall rule "reads only the CPU of claude's own process group". AR83's: "no ledger line counts a sitting's result events"; a second result that "already holds the first query" would be counted twice.

The probes (2.1.302, `claude-haiku-5-5`, `-p --permission-mode auto`, stream-json, no `CLAUDE*` variable):
- `echo`, `sudo -n true` and `git push --dry-run origin main` each ran, `permission_denials` `[]`, with `--allowedTools` a bare `Bash`, `Bash(<cmd>:*)` or absent. Nothing was refused: the bare entry's effect is not measured.
- A child that backgrounded `sleep 75` wrote its first `result` (`num_turns` 2, `duration_ms` 4208, $0.00452667) and exited 75 s later, the sleep gone. Between: `ui_invalidate` every ~5 s, task events, a second `system/init` (same session), one message, a second `result`: `result_index` 1, `origin.kind` `task-notification`, `num_turns` 1, `duration_ms` 1324, $0.00499084; its `usage` its own query's, its `modelUsage` both results' `usage` summed. The fixture is that stream, its paths and three sibling names redacted as `testdata/real.jsonl` is.

The code: `DefaultAllowedTools` (`internal/effects/config.go:56`). `Tally.Add`: `system/init` sets session and model (`stream.go:220-222`), `result` folds (`:269-282`). `RunRow`: the wall (`run.go:656`), the watchdog (`:673-708`; CPU over `Procs.Group` alone, `bound.go:1096`), the result's log line (`:743`), the `Wait` (`:783`), the sweep (`:796-798`), the `exit` line (`:831-855`), the gate (`:878`).

## 2. Rulings

Standing rules. The charter, `history/sittings/<date>-<n>-ar90.md`, opens at act 1 and closes before the last commit; no home path. Each act's first test is red on the pin, its red line in the commit message; act 2's is a proof, green on the pin, and its message says so. `DECISIONS.md` stays as it is.

1. **Act 1, the spawn line.** `DefaultAllowedTools` keeps its bare `Bash`: the docs say auto mode drops it and the 2026-10-16 probes could not measure it, but where auto mode is unavailable and the session falls back to Manual, the entry is what pre-allows a headless child's Bash calls, which would otherwise be denied. The comment names the measurement, the CLI version and that reason. A configured `allowed_tools` passes as written. `TestAR90_1_BareBashKept` (effects): one word of the default is `Bash`; `LoadConfig` with no file gives it; a user file's `allowed_tools: "Bash(go test:*) Edit"` reads as written. The entry's measurement stays open (ruling 5).
2. **Act 2, the real wait.** `TestAR90_2_RealBackgroundWait` (stream), `parseFile` of the fixture: `Turns` 3, `NonJSON` 0, `SessionID` `fb524734-4f4a-4082-bd17-ee43185daaa5`, `Model` `claude-haiku-5-5`, `Account` empty; `Result.NumTurns` 3, `DurationMS` 5532, `Denials` 0, `CostUSD` and `Tally.CostUSD` 0.00499084, `IsError` false, `Subtype` `success`, `ResultTail` the second's text; `Usage` input 6, cache read 81224, cache create 20565, output 130, equal to the last result's `modelUsage` as the test reads it; `Context` 34417. On 2.1.302 turns, time and usage are per query, the cost a running total: the fold counts no query twice.
3. **Act 3, the rules against a waiting child.** Today no rule reads a `result` as the end: it logs; the gate follows the `Wait`, which returns after the wait. The wall, armed at the spawn, bounds the wait (`wall-clock`). The stall rule reads HEAD, the newest mtime and the child's group's CPU; a tool call leads its own group (§15), so a background command's CPU is unread; §15's 10-minute `-p` wait is under 2,700 s. The sweep follows the exit, by when the CLI had waited (the sleep was gone). No change to the three. One record change: `Tally.Results` counts result events, and the `exit` line carries `results`. `TestAR90_3_ResultsCount` (stream): 0 for `noresult.jsonl`, 1 for `ok.jsonl`, 2 for `two-results.jsonl` and the fixture. `TestAR90_3_WaitedChildGreen` (night): `RunRow` over `FakeSpawner{Out: the fixture}`, no survivor, `greenGate`: green; the `exit` line's `results` 2, `turns` 3, `duration_ms` 5532, `cost_usd` 0.00499084, no `reason`; `orphans` 0; two result lines logged. `TestAR90_3_WallBoundsWait` (night): a spawner that writes the fixture to its first `result`, then holds stream and `Wait` until signalled, `Wall` 300 ms: failed `wall-clock`, `results` 1, `turns` 2.
4. **Act 4, the record.** relay-r1 §5: `results`, act 2's reading. §7: the bare `Bash` kept, the measurement and the Manual-fallback reason. §15: re-read on 2.1.302 (2026-10-16), the probe's numbers, ruling 3, the version the rules are tuned on. §11: the five tests. NICE: `(AR90)` on §4's items.
5. **Left, filed in act 4 (S4).** The bare entry's effect in auto mode, which needs a command the classifier refuses (the probes' three were all allowed); §15's 10-minute wait against the audit's 2 h, not probed; claude's own CPU in a wait; a kill mid-wait leaves a background shell in its own group, which `Sweep` misses and `SweepEscaped` takes only if a 60 s sample saw it (candidate: a session sweep, the child leads its session); a `ScheduleWakeup`, background `Agent` or `Monitor` can hold the exit to the wall.

## 3. Interfaces (signatures, not prose)

`effects`: `DefaultAllowedTools` unchanged, its comment new. `stream`: `Tally.Results int`, the result events `Add` read. `night`: the `exit` line's `results` (an int, always written). No new event, flag or function.

## 4. Findings that name this row

NICE "The research site's reading of Claude Code" (2026-10-14), items 3 and 1 (S3); "The evidence behind rf1's permission and background-wait items", its two S4s; "AR83's build: what it leaves"; "AR83's review", its second S4.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory; the probe's raw streams. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: each act's red line; rulings beyond §2; anything filed. Last line: branch, head sha.
