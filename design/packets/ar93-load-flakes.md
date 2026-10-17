# Packet AR93: the two load flakes are order races; each test waits for the event it races

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`, "The full check's load flakes, the second sighting" (S3), "AR79's review" (its last S3), AR35's line (under "Moved from example-mac's"), "AR84's build" (its first S4); `DECISIONS.md` entries 2 and 10 (§1); `design/packets/ar35-load-tolerant-tests.md` §2; `design/relay-r1.md` §11; the survey `research/synthesis/tools-relay.md` in agentik-research, §1.3; the code at 28c4e2a |
| pin | `main` at 28c4e2a plus this packet's own commit; a linked worktree, branch `ar93-load-flakes` |
| wall | AR83's: this repo only; no push, `--amend`, rebase, device, ssh or network; no `relay` binary by hand or outside `make`'s harness; no `relay` or `agentik relay` verb on the account's state; never `claude`. No `make test`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/night`, `gofmt -l internal`, `make doclint`, `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. One `make check`, last, started only once `ps -axo command \| grep -E '(^\|/)make( \|$)' \| grep -v grep` prints nothing, and waited for. Never spell the Go name of the local zone. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/effects/testdata/fakeclaude/main.go` (the mid-act commit's wait, the header's knob sentence), `internal/night/ar28_test.go` (`TestAR28_1_TuneOnExit` and its helper only), `internal/night/ar81_test.go` (`TestAR81_4_QueuedBehindNewHead` and its helper only), `internal/night/ar93_test.go` (new), `design/relay-r1.md` (§11's test list only), `NICE_TO_HAVE.md` (two marks; one new section), `history/sittings/*.md`, `history/sessions/NEXT`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR93_' -race -count=3 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR93_'` reads 6 (main: 0); `go test ./internal/night -run '^Test(AR28_1_TuneOnExit\|AR81_4_QueuedBehindNewHead)$' -race -count=10 -timeout 300s 2>&1 \| grep -c -E '^ok '` reads 1 (main: 1); `go test ./internal/night -run '^Test(AR28_\|AR30_3\|AR81_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 (main: 1) |
| budget | 70 turns, 180k context, `claude-opus-5-5`; three acts, one commit each; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says (quoted)

NICE (S3, 2026-10-16): "`TestAR28_1_TuneOnExit/commit` read `commit_turn:4`, want 5, twice today in `make check`'s parallel race shards (… load 7 to 9 with two builders and a night), 3 of 3 and 2 of 2 green alone each time; `TestAR81_4_QueuedBehindNewHead` read `waited 10 s for r2's ask` once (19:27), 2 of 2 green alone." (The hq sitting counts three reds of the first that day.) The first sighting (2026-10-14): "a second sighting of any timing test would argue for fewer shards or `-p` beside them." AR35: "made the five timing tests that flaked under load hold on a loaded machine"; its packet's ruling 4: "wait for the fake's own signal, a file or a pipe it writes, with a generous deadline, not a fixed count of short attempts"; ruling 5: "No `t.Skip` under load, and no reading of the load average. Do not delete an assertion." The S4 (AR84's build): "listen on port 0, close, use … may hand two of `make test`'s night shards one port … a flake not yet seen."

`DECISIONS.md`, verbatim. Entry 10: "A repo's relay config may carry its own verify limit; when it is unset, the limit stays 900 s. `agentik relay doctor --verify` measures the repo's verify line and suggests a limit. A verify line cut at its limit reads red with a reason that says the line was cut, never with an empty reason." Entry 2, its last sentence: "Every packet follows relay-r1 §2, its writer's appended notes included: under 9,216 bytes, six sections, and acceptance on the allowlist." The survey (§1.3), of an earlier flake: "The fix waits on a condition, not a longer sleep".

The code at 28c4e2a. AR28_1's commit case waits for nothing: the fake (`fakeclaude/main.go:120-123`) commits at line 5 before that line's 150 ms sleep (`:131`), so the moment turn 4's line is flushed; the relay reads HEAD at each fresh turn after the soft one (`run.go:771-775`). Turn 4's `git rev-parse` races the fake's `git commit` with no margin, and reads 4 when it lands second; load delays the read. No timeout is involved. AR81_4 polls 10 s (`ar65Until`) for r2's ask, but `ar80Ask` steps the clock to +120 s once the fake band has recorded ask 2 (`band.go:264`), while r1's `leave` reads the clock afterwards (`run.go:446-448`, `:249-255`): a step that lands first sets r2's earliest ask to +180 s, a clock the test never reaches, so any wait fails. The step after ask 4 races the same way.

Measured. Alone, 40 of 40 green on the pin (each test `-race -count=10`, plain and `GOMAXPROCS=1`, load 7 to 9; 10 more each at load 25). Forced: head reads 300 ms late after the soft line give `commit_turn:4` 3 of 3; the band answering 50 ms late gives `waited 10 s for r2's ask` 3 of 3, and with only the first wait added `asks r1 +0s, r1 +60s, r2 +120s, r2 +180s`; with §2's waits, 3 of 3 green under each.

## 2. Rulings

Standing rules. The charter, `history/sittings/<date>-<n>-ar93.md`, opens at act 1 and closes before the last commit; no home path; it records acceptance line 2 at its open. Each act's first test is red on the pin, its red line in the commit message. `DECISIONS.md` stays as it is. AR35's ruling 5 binds; no product code or production value changes.

1. **The device** (AR35's ruling 4). Neither red is a deadline: each is an order between the test and a goroutine or child that load inverts, so each test waits for the event that orders it, under a hang guard. The old names keep every assertion.
2. **Act 1, the commit after the read.** The fake gains `FAKECLAUDE_COMMIT_AFTER=<path>`: its mid-act commit waits until the path exists, 30 s at most (under the row's minute wall), then commits as now; unset, nothing changes (AR30_3's use). `ar93SlowHead` (`effects.OSGit` embedded) delays each `Head` by `slow` once the ledger holds the `soft` line, then touches `done`. `TestAR28_1_TuneOnExit` runs its table through a helper at `slow` 0, `done` named to the fake; `TestAR93_1_CommitAfterTheRead` at 300 ms. The header names `_COMMIT_AT` and `_COMMIT_AFTER`.
3. **Act 2, the step after the leave.** Before each step past an admitting ask (+120 s after ask 2, +240 s after ask 4), the test waits (`ar65Until`) for that row's `admitted` band line, which `askBand` writes after its `leave`. `ar93LateBand` (`*effects.FakeBand` embedded) answers each ask `lag` after the record; `TestAR81_4_QueuedBehindNewHead` runs through a helper at 0, `TestAR93_2_StepAfterTheLeave` at 50 ms.
4. **The shards stay** (AR84): both races sit inside one test in one process and opened under load from outside the check; fewer shards narrow them, close neither. No `-p` cap.
5. **The S4 lock port stays**: a port held until the first night must still free it for the re-executed children that bind it (`lockPortEnv`), and a range per shard needs the Makefile to number its shards; neither is under ten lines with a test, and neither red was lock contention.
6. **Act 3, the record.** relay-r1 §11: the lines of `TestAR28_1_TuneOnExit` and `TestAR81_4` gain their order (AR93); the two `TestAR93_` lines. NICE: the second sighting and the 10-14 S3 gain `(AR93)`; a new section "AR93's build (2026-10-16): what it leaves" (S4): each other `ar80Ask` step that follows an admitting ask from a held row, named, unchanged as unseen; ruling 5's reasons.

## 3. Interfaces (signatures, not prose)

Test-only. The fake: `FAKECLAUDE_COMMIT_AFTER` (a path). `night_test`: `ar93SlowHead{effects.OSGit; w *world; slow time.Duration; done string}`, `ar93LateBand{*effects.FakeBand; lag time.Duration}`, the two helpers (`slow`, `lag`), `TestAR93_1_CommitAfterTheRead`, `TestAR93_2_StepAfterTheLeave`. No product code, `--json` key, flag, event or exported name.

## 4. Findings that name this row

NICE "The full check's load flakes, the second sighting" (S3); "AR79's review", its last S3; "AR84's build", its first S4 (left, ruling 5).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo (§1 quotes the survey).

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count; the exits of `go vet`, `gofmt -l`, `make doclint` and `make check`; `make hygiene`'s last line. Bullets: each act's red line; line 2 at the open and at the end; choices beyond §2; anything filed. Last line: branch, head sha. Fork for the owner: none now; it opens only if a timing test reds in the sharded check after AR93 at a load under 4 per core: fewer shards under load, or a `-p` cap.
