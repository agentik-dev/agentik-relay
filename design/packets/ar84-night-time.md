# Packet AR84: the night package's tests finish under 300 s

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`'s "The observer's check-in 120" (2026-10-14), its S3 item on `internal/night`'s time; `design/relay-r1.md` §11 (`go test -race ./...` in `check`); the observer's run of 18:09 and the cutter's measurements (§1); the code at e06e1aa |
| pin | `main` at e06e1aa plus this packet's own commit; a linked worktree, branch `ar84-night-time` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make test`, `make install`, `make readme`, `make e2e`, `go test ./...` or `internal/night` unfiltered: the acceptance lines (the second once, when `pgrep -x make` prints nothing), `make -n test`, `go vet ./internal/night`, `gofmt -l internal/night`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `Makefile` (the `test` rule alone), `internal/night/testdata/shards.txt` (new), `internal/night/ar84_test.go` (new), `design/relay-r1.md` (§11's `check` sentence and test list only), `NICE_TO_HAVE.md` (the S3 item's mark; a new section for what the build leaves), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR84_1_' -count=1 -timeout 120s -v 2>&1 \| grep -c -E '^--- PASS: TestAR84_1_'` reads 1; `go test ./internal/night -run '^TestAR84_2_' -count=1 -timeout 720s -v 2>&1 \| grep -c -E '^--- PASS: TestAR84_2_'` reads 1; `go test ./internal/night -run '^Test(AR84_\|RunOneRowFake\|NightEndToEnd)' -count=1 -timeout 300s 2>&1 \| grep -c -E '^ok '` reads 1 |
| budget | 50 turns, 150k context, `claude-opus-5-5`; two acts, one commit each |
| verify | `make check` |

## 1. What the source says (quoted)

NICE (S3): "`internal/night` runs its 308 tests in series (no `t.Parallel`) for 522 to 602 s under `-race` ... Shards in separate test processes, or a cheaper per-test fixture, would bring it under 300 s." The observer's `-race -v` run: 602 s; 30 tests at 5-7 s (173 s), 13 over 7 s (119 s); `make check` 656 s against `scripts/bounded --wall 840`.

The cutter's measurements at e06e1aa (16 cores, load 5 to 45). A row costs about 1 s of child-process latency, not CPU: `TestRunOneRowFake` 1.07 s is `newWorld` 40-90 ms, `Stage` 140-190 ms, the fake child 20-160 ms and the gate 520-800 ms: about 26 git runs through `/usr/bin/git`'s shim (6.7 ms each, 3.3 ms direct), the line 140-150 ms (the session sweep's `lsof -t`, `effects/procs.go:255`, 105-112 ms, 76 of them kernel; two `ps`), the fresh clone 120 ms, `ToolsMoved`'s walk 70 ms. Go's trace puts 87% of the blocked time in `wait4`; the process holds 1.2 cores. A 5-7 s test is 5 to 7 rows (`TestAR45_1`: seven one-row subtests); no real-clock wait leads (the night tests poll at 20 ms). Five groups at once (rest, AR1, AR2-3, AR4-5, AR6-9) took 116, 68, 187, 142 and 265 s, the tests' sum 767 s (AR32's family 110 s). The four shards of ruling 2, run at once by a prototype of `TestAR84_2`: 175, 206, 203, 209 s, 251 s user and 463 s system: the kernel's process work caps more shards (six would save about 22%).

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar84.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is. No existing test, helper or product file changes, so no test's assertion moves.

1. **Shards (b).** (a) buys nothing without a rewrite: `newWorld` calls `t.Setenv` six times and `fakeClaude` once (a parallel test may not), `newWorld` swaps `night.LockDir`, `LiveClaude` and `KillGrace`, 12 files swap more (`LockPort`, `PinTools`, `PipeGrace`); 138 tests sit in files that build a world. (c) cannot reach 300 s: the fixture's own share is under 0.15 s a row; the rest is the product's effects, and a test-only cut (a direct git first in `PinDirs`, about 0.2 s a row) tests a git production never pins. (b) needs no test edit: each shard is its own process and `TestMain` (state, `HOME`, pin, lock port). Expected: about 210 s here, from 602.
2. **Four shards by the AR number's last digit**, balanced on the measured times, in `internal/night/testdata/shards.txt`, one `-run` pattern a line: `^Test([^A]|A[^R]|AR[^0-9]|AR[0-9]*[36][^0-9])`, `^TestAR[0-9]*[045][^0-9]`, `^TestAR[0-9]*[189][^0-9]`, `^TestAR[0-9]*[27][^0-9]` (74, 89, 69, 76 tests). A new packet's tests land by its number, so the shards fill in turn; a name with nothing after its digits matches none, and `TestAR84_1` names it.
3. **The file, not the `Makefile`.** The gate judges a row under `main`'s `Makefile` (ADV10-10), so this row's verify runs the old rule and its tests must pass under either. The new rule first runs in the observer's `make check` after the merge.
4. **`make test`** runs every package but `internal/night` in one `go test`, beside one `go test -run <line>` of `internal/night` per line, all at once, and exits non-zero when any does (§3). The 15 min timeout stays, a ceiling.
5. **Tests.** `TestAR84_1_ShardsPartition`: the file ends in a newline; each line starts `^Test`, holds no `/` or blank, compiles; two lines at least; every top-level `func TestX(t *testing.T)` in the package's `*_test.go` (`go/parser`; at least 300) and the planted `TestAR99_1_X`, `TestAR100_1_X`, `TestAR7Fix_X`, `TestZed`, `TestAgentikR2_1_X` each match exactly one line, else an error naming it and its shards. `TestAR84_2_ShardWalls` skips unless `-run` reads exactly `^TestAR84_2_`, so a shard never starts it; it builds `go test -c -race` into a temp dir, starts one process per line at once (`-test.run <line> -test.count=1 -test.v -test.timeout 15m`, cwd the package, the env without `AR22_2_NIGHT_LOCK_PORT`, its own group, killed at 300 s) and logs each wall and count; each exits 0, their top-level `--- PASS`/`--- SKIP` lines number `TestAR84_1`'s names, the slowest under 300 s.
6. **Act 2, the record.** relay-r1 §11: `check` runs `internal/night` as the shards of `testdata/shards.txt` at once (AR84); the list gains both tests. NICE's S3 pointer becomes `(AR84: ...)` with the measured walls.
7. **Left, filed in act 2** (S4): the lock-port pick (`ar22_2_test.go:28`: listen on 0, close, use) may give two shards one port, as for the gate and cmd binaries today; `lsof -t` (110 ms a line) is a row's largest single cost.

## 3. Interfaces (signatures, not prose)

`night_test`: `func shardPatterns(t *testing.T) []string`, `func testNames(t *testing.T) []string`; `flag.Lookup("test.run")`. `Makefile`'s `test` body, its comment naming AR84:

```
pids=; go test -race -timeout 15m $$(go list ./... | grep -v '/internal/night$$') & pids=$$!; \
while read -r p; do go test -race -timeout 15m -run "$$p" ./internal/night & pids="$$pids $$!"; done < internal/night/testdata/shards.txt; \
rc=0; for p in $$pids; do wait $$p || rc=$$?; done; exit $$rc
```

No new exported name, ledger line, key or event.

## 4. Findings that name this row

NICE's S3 item in "The observer's check-in 120" (2026-10-14). Near it: relay-r1 §6's measured tree walk (`make check` rose from 93 s to 150 s).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line; `TestAR84_2`'s four walls. Bullets: each act's red line; rulings beyond §2; anything filed. Last line: branch, head sha.
