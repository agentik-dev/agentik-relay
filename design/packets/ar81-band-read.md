# Packet AR81: status, report and doctor read the band's hold

| item | value |
|---|---|
| from | `design/band-pacing-r1.md` (c63f5d3) §3 (`status`, `--json`, `report`, `doctor`), §5 packet 2; packet AR80's lines (its rulings 4 to 7); the code at 7450c05 |
| pin | agent-relay `main` with AR80 merged, plus this packet's own commits; a linked worktree, branch `ar81-band-read` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness; never run `agentik-session`. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet` and `gofmt -l` of the three packages, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/night.go` (the tap's `Append`, `launch`), `internal/night/run.go` (`RunOptions.asked` and its one call in the hold), `internal/night/status.go` (`RowStatus`, `RowBand`, `buildStatus`, `PrintStatus`), `internal/night/report.go` (`Report`, `BuildReport`, `Markdown`), `internal/night/testdata/json-keys.golden`, `internal/night/ar81_test.go` (new), `internal/effects/band.go` (`BandVersion`), `cmd/agentik-relay/main.go` (`doctor`, `bandVersion`), `cmd/agentik-relay/ar81_test.go` (new), `design/relay-r1.md` (§9, §11), `design/band-pacing-r1.md` (Status), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR81_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR81_'` reads 5; `go test ./cmd/agentik-relay -run '^TestAR81_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR81_'` reads 1; `go test ./internal/night -run '^Test(AR47Fix_\|AR51_\|AR62_\|AR77_\|AR80_\|AR81_\|Report\|Status)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./cmd/agentik-relay -run '^Test(JSONShapesFrozen\|AR4_JSONKeysSnake\|DoctorPrinted\|NightVerbs\|AR75_7_\|AR81_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

band-pacing-r1 §3: "`status` (`status.go:25`, `:47`): a held row reads `held` (display only), its reason the band's text; `--json` gives it `band: {verdict, peak, since, asks}`, and `next` leads with `the band holds <rows> since HH:MM`. `report` counts holds and their wait. `doctor` shows the file and its `--version`, `absent` or `off`, never asking (an ask writes a verdict line)." §5: "**AR81, the read.** §3's `status`, `--json`, `report` and `doctor`, by named tests over a held night's ledger." P2: "`agentik-session --version` takes 0.05 s". §6: "A broken plugin turns pacing off, seen only in `unchecked` lines and doctor".

AR80's lines: a hold's `held` line (`verdict`, `peak`, `text`; a queued row's `verdict: queued`, `text` `behind <head>`), one more at a hold's first `denied`; its end, `admitted` or `reset` (`asks`, `wait_s`); `unchecked` (`error`; in a hold `asks`, `wait_s`); `sitting.band` `{asks, wait_s, verdict}`. A re-ask writes no line; a hold that a stop, a kill, `--until` or a sibling's limit ends writes only its `sitting` line.

The code at 7450c05: `RowStatus` `internal/night/status.go:47`, `buildStatus` `:229` (`next` `:245-261`), `PrintStatus` `:272`; the tap `night.go:281`, its `Append` `:305`, `launch` `:762`; `Report` `report.go:23`, `BuildReport` `:125`, `Markdown` `:472`; `doctor` `cmd/agentik-relay/main.go:200`, its `caffeinate` check `:228-234`; the golden's status rows `internal/night/testdata/json-keys.golden:165-183`, report `:101-131`, read by `TestAR47Fix_GoldenFreezesBreakerKeys` and `TestJSONShapesFrozen` (`main_test.go:652`).

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar81.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is. `json-keys.golden` is refrozen with ruling 7's keys alone.

1. **Act 1, status** (the tap's `case "band"`). A `held` line sets the row's `state` `held` (display only: the sitting stays `running`), its `reason` the `text`, and `band` `{verdict, peak, since, asks}`: `since` the line's time (RFC 3339, the clock's zone), `asks` 1, or 0 when `queued`. A second `held` line updates all but `since`. An end or `unchecked` line, or the part's `spawn` or `sitting` line, sets `band` null, the reason "" and the state back to the sitting's.
2. **The asks between lines.** AR80 writes no line per re-ask, so `RunOptions.asked` (unexported, set at `launch`) is called after each re-ask with its verdict; under `lg.mu` it adds one to `asks` and sets `verdict`, `peak` and the reason. The next status write shows it.
3. **`next` leads** with `the band holds <rows> since HH:MM`: the held rows in `ids` order, the earliest `since`; no clause when none is held. A terminal or draining night's `next` stays as today.
4. **PrintStatus** prints a held row as any row: `held` in the state column, its reason clipped under 100 columns.
5. **Act 2, the report.** `Report.Band` is `{holds, wait_s}`. A hold is a `(row, part)` with a `held` line; its wait is the first of its end line's `wait_s`, its `unchecked` line's, its `sitting` line's `band.wait_s`; an open hold counts, with no wait yet. `Markdown` gains one line after the sittings, `band: <holds> holds, <d> waited` (`d` a `time.Duration`'s String), only when holds > 0.
6. **Doctor**, after `caffeinate`, never asking: `off`: ok, `off: the config turns pacing off`; absent: ok and warn, `absent: agentik-session not on PATH; spawns unpaced`; present: ok, `<file>: <--version's first line>`, home as `~`; a failing `--version`: ok and warn, `<file> --version: <err>; spawns unchecked`. A broken plugin warns and never fails doctor: the night fails open. `effects.BandVersion(file)` runs `<file> --version` under `BandTimeout`; doctor calls it through `bandVersion`, a cmd variable, after `bandFor` (AR80). A config error reads as `pace`.
7. **The golden** gains `report band`, `report band.holds`, `report band.wait_s`, `status rows.[]band` and its `.asks`, `.peak`, `.since`, `.verdict`; nothing else moves.
8. **Tests.** `TestAR81_1_StatusHeld` (`TestAR80_5_QueueOneAsks`'s world): status.json at T: r1 `held`, its text, `{not-started, its peak, T, 1}`; r2 `held`, `behind r1`, `{queued, null, T, 0}`; `next` leads `the band holds r1 r2 since 01:00`; at +60 s r1's `asks` 2; at +120 s r1 `running`, `band` null, `next` leading `the band holds r2 since 01:00`; PrintStatus's lines under 100 columns, r1's `held`. `TestAR81_2_ReportHolds` (ledger lines, no git): r1 held 120 s then admitted; r2 held, then skipped by a stop (`sitting.band.wait_s` 30); r3 `unchecked` at its first ask; r4 held and open: `band` `{3, 150}`, the line `band: 3 holds, 2m30s waited`; a night with no `band` line: `{0, 0}`, no line. `TestAR81_3_DoctorBand` (cmd; `bandFor` and `bandVersion` faked): present (`agentik-session 2.0.0-alpha.14`), `off`, absent, a failing `--version`: ruling 6's ok, warn and detail, doctor's exit 0 each time, the `FakeBand` never asked.
9. **Act 3, the record.** relay-r1 §9: a held row, `next`'s clause, the report's count, doctor's check; §11: the three tests. band-pacing-r1's Status: "AR81 built §5 packet 2".

## 3. Interfaces (signatures, not prose)

`night`: `RowBand{Verdict string; Peak *float64; Since string; Asks int}` (json `verdict`, `peak`, `since`, `asks`); `RowStatus.Band *RowBand` (`band`); `Report.Band` `{Holds int; WaitS int}` (`band`: `holds`, `wait_s`); `RunOptions.asked func(effects.BandVerdict)`. `effects`: `BandVersion(file string) (string, error)`. `cmd`: `var bandVersion = effects.BandVersion`. `BuildReport`, `buildStatus` and `PrintStatus` keep their signatures.

## 4. Findings that name this row

None in `NICE_TO_HAVE.md` (band, pacing, admission, band-target; grep at 7450c05). Near it, open, S4: AR77's build note that `nightBurn` counts a part that staged and never spawned (a hold a stop ends is one); AR75's review note that `doctor` finds its root by `toplevel()`, not `Git.Root` (the `band` check reads that `cfg`). band-pacing-r1 §6: a relay hold is never in `band.json`'s held count (the plugin's).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state and agentik-session's. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: each act's red line; each existing test changed; the golden's added keys; anything filed. Last line: branch, head sha.
