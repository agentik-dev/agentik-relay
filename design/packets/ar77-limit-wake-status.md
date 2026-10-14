# Packet AR77: after a usage-limit wake, status and report read the row's new part

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`'s section "After a usage-limit wake, status and report show the dead sitting" (078e6e5, night 2026-10-13-7); `design/usage-limit-resume-r1.md` §2 (the sitting line, the halt, the wake) and §5 packet 6, its status and report half; `internal/night/night.go`, `status.go`, `report.go` at 925f104 |
| pin | agent-relay `main` at 925f104 plus this packet's own commit; worktree `.claude/worktrees/ar77-limit-wake-status`, branch `ar77-limit-wake-status` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/night`, `go test ./internal/night -run '^Test(AR55_\|AR56_\|AR62_\|AR66_\|AR69_\|AR70_\|AR71_\|Status)' -count=1 -timeout 600s`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/night.go` (`continueRow` only), `internal/night/status.go` (`nightBurn` and `buildStatus`'s `next` clause, with their comments), `internal/night/report.go` (`BuildReport` and its comment only), `internal/night/ar77_test.go` (new), `design/relay-r1.md` (§9's paragraph "Status and the report count one night the same way" and §11's test list only), `design/usage-limit-resume-r1.md` (its Status line only), `NICE_TO_HAVE.md` (the section's heading; a new section for what the build leaves), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR77_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR77_'` reads 3; `go test ./internal/night -run '^Test(AR51_\|AR65_\|AR67_\|AR68_\|AR74_\|AR77_\|Report)' -count=1 -timeout 600s -v 2>&1 \| grep -c -E '^(--- FAIL\|ok )'` reads 1 |
| budget | 70 turns, 150k context, `claude-opus-5-5`; one act, one commit |
| verify | `make check` |

## 1. What the source says (quoted)

usage-limit-resume-r1 §2: "A limited part's `sitting` line stays `failed: usage-limit` and gains `limit: paused` when the night pauses on it. `redSittings` … skip that line … A night that then stops names the row in its `stop` line's resume lines". "A night that ends paused halts each paused row `failed: usage-limit` (`Step(Paused, Halt)`)". "A continuation is not a pick". §5 packet 6: "The readers … status, report, the stop verb's line, the attendant". relay-r1 §9: "`--max-sittings`, the `stop` line's `sittings` and status's `next` count only rows that staged (AR58 ruling 3)."

The ledger (night 2026-10-13-7). At 06:26:19Z three `sitting` lines read `failed`, `usage-limit`, `limit: paused`: po1bfix (1 turn, exit 143), po2fix and us3afix (0 turns; staged, never spawned). One `pause` line (count 1, wake 00:32 PDT). At 07:32:00Z a `wake` line names the three; part 2 spawns at 07:32:00Z, 07:32:30Z and 07:33:00Z; the first `turn` line is po2fix's, 10 turns and 61225 (07:33:33Z, the observer's "10/61k").

The code at 925f104. `continueRow` steps `Spawn` and keeps the sitting's reason, which `setRow` copies at `launch`; nothing resets the row's counts until its first `turn` line, every tenth turn. `BuildReport` over the night's lines to 07:33:40Z (a probe under `go test`) gives no stopped row and no resume line (the `spawn` takes back, AR74), but its burn reads po1bfix, po2fix and us3afix `failed` above their three `running` parts: the report's `failed`. While paused it lists each paused row `failed` with a resume line; status reads it `paused`, no resume. `burn.sittings` read 6 (three dead parts and three in flight), `next`'s count 3.

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar77.md` in doclint's five-part shape, opened at the act, closed before the commit, no home path. The act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is: entry 12's scope is untouched (a row on another model is never paused, so never continued), and entry 11's fallback (packet 7, unbuilt) is a continuation that ruling 1 covers when it lands. No `--json` key is added or removed and `json-keys.golden` stays (AR59 act 3's rule): only values change.

1. **Status: the continuation is the row's sitting.** In `continueRow`, after `Spawn`, the sitting's reason is `""`. Under `lg.mu` the row's `turns`, `context`, `cost_usd`, `unread`, `last_tool`, `builders` and `cause` go to their zero values; the part's own lines fill them. `elapsed` counts on from part 1's start (AR67's F1), `acts` stay, `resume` stays `""`; the tap's spend (`usd`, `held`) is untouched, so `--max-usd` counts every part.
2. **Status: `next` names its count.** The clause reads `count-reached after <N> rows staged (<M> so far)`: rows that staged (AR58 ruling 3), while `burn.sittings` counts parts.
3. **Report: a paused line is no verdict.** A `sitting` line with `limit: "paused"` takes back its row's earlier verdict as any `sitting` line, then holds the row paused: no `Stopped.Rows` entry, no resume line, not green (as `redSittings`, AR66 ruling 3). The row's next `spawn` or `sitting` ends the hold, as today. The night's `stop` line halts each row still held, in the order their paused lines came: an entry `failed`, `usage-limit`, its `cause` the paused line's, and its resume line (entry and `Resume`) only when the `stop` line's `resume` names the row (`relay.ResumeRow`; `[]any` or `[]string`), so a sitting the wake halted (AR68) gets none, as status. HandMerges' clearing runs after, unchanged.
4. **Report: the burn keeps the paused part as history.** In `nightBurn` a `sitting` line with `limit: "paused"` gives its entry the state `paused`; the night's `stop` line turns an entry still `paused` whose row spawned no part after it to `failed` (the halt). After a wake the burn reads the dead part `paused`, then the part `running`, then its verdict. Status's `burn` total is unchanged.
5. **The tests**, in `internal/night/ar77_test.go` (`night_test`). `TestAR77_1_StatusReadsThePart`: TestAR67_3's "elapsed while part 2 runs" world, `MaxSittings` 5; paused, r1 reads `paused`, `usage-limit`, 40 turns, $11.50; at the wake, part 2 held: r1 `running`, reason `""`, turns, context and cost 0, unread false, last tool `""`, builders 0, cause `""`, resume `""`, elapsed `1h12m0s` at +1h12m, `next` holding `count-reached after 5 rows staged (1 so far)` while `burn.sittings` reads 2; released, r1 `green`, 3 turns, $1.25. `TestAR77_2_ReportWhilePaused`: ledger lines, no git: r1 spawned and r2 not, each a paused `sitting` line, then a `pause` line: no stopped row, no resume, burn `paused paused`, the Markdown `0 stopped short`, no `failed`; then a `stop` line whose `resume` names r1: r1 `failed`, `usage-limit` with its line, r2 `failed` with none, `Resume` r1's alone, burn `failed failed`. `TestAR77_3_ReportAfterTheWake`: the night's shape from §1 to 07:33:40Z: no stopped row, no resume, burn `paused` thrice then `running` thrice, `6 sittings (3 in flight)`, no `failed`; then its part 2 lines (po2fix and us3afix red `verify-cut`, po1bfix green) and a `stop` line naming po2fix and us3afix: each red with one resume line, po1bfix waiting, burn `paused paused paused red red green`. TestAR77_1 is the cited red on the pin (reason `usage-limit`, 40 turns in part 2); _2 and _3 are red too.
6. **The record.** relay-r1 §9's paragraph gains one sentence citing AR77 (rulings 1 to 4); §11 lists the three tests. usage-limit-resume-r1's Status line gains "AR77 built §5 packet 6's status and report half". The NICE heading gains `(AR77)`.

## 3. Interfaces (signatures, not prose)

`BuildReport(lines []map[string]any, night string, g ...effects.Git) Report` and `nightBurn(lines []map[string]any, night string) (rows []Burn, total Burn)` keep their signatures. No exported name, ledger line or `--json` key changes.

## 4. Findings that name this row

`NICE_TO_HAVE.md`'s "After a usage-limit wake, status and report show the dead sitting" (the observer, 2026-10-14; S4). "AR74's report-lens review", its woken orchestrator sitting's resume line (S4): ruling 3 closes the report's half. Packet 6's rest stays open: the stop verb's line, the attendant, `relay run --json`'s resume line for a woken sitting ("AR68 rounds one and two"), a model's scope in status and report (AR71 ruling 8).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory (§1 quotes the ledger). Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: TestAR77_1's red line on the pin; each existing test changed; anything filed. Last line: branch, head sha.
