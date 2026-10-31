# Packet AR98: a `none` login's denial stops the night; the answer's reset; a `wait` with no reset holds

| item | value |
|---|---|
| from | agentik-hq `HANDOFF.md` item 14 (S3); a private hq's design record, quoted in §1; `DECISIONS.md` entries 12 and 15 (§1); band-pacing-r1 §2; the session plugin's admission at 2.0.0-alpha.29 |
| pin | `main` at 57aba66 plus this packet's own commit; worktree `.claude/worktrees/ar98-window` |
| wall | this repo only; no push, `--amend`, rebase or network; never build a relay binary by hand, never run one outside `make`'s test harness, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; never `claude`. No `make check` (the coordinator runs it), `make test`, `make install`, `make readme`, `go test ./...` or `./cmd/...`: the acceptance lines, `go vet`, `gofmt -l internal`, `make doclint`, `make hygiene`, one at a time at `nice -n 10`. Before the report `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/effects/band.go`, `internal/effects/ar98_test.go` (new), `internal/night/run.go` (`askBand`, `bandEnd` and `unspawned`'s path), `internal/night/plan.go`, `internal/night/night.go` (only the stop a `bandWeekly` end needs), `internal/night/ar98_test.go` (new), `internal/stream/limit.go` (`eventKinds`), `internal/stream/ar98_test.go` (new), `design/band-pacing-r1.md` (its Status line), `design/relay-r1.md` (§15), `design/packets/ar98-window.md`, `history/sittings/*.md`, `history/sessions/NEXT`, `NICE_TO_HAVE.md`. Nothing else; `internal/night/plan.go`'s callers in `cmd/` only for a printed line, named in the report |
| acceptance | `go test ./internal/night -run '^TestAR98_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR98_'` reads 3 (`_1`, `_4`, `_5`); `go test ./internal/effects -run '^TestAR98_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR98_'` reads 1 (`_2`); `go test ./internal/stream -run '^TestAR98_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR98_'` reads 1 (`_3`); `go test ./internal/night -run '^TestAR(65\|69\|80\|81\|85\|87\|92\|93\|97)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./internal/effects ./internal/stream -count=1 -timeout 300s 2>&1 \| grep -c -E '^ok '` reads 2; `git diff --stat main -- internal/night/ar80_test.go internal/night/ar65_test.go internal/night/ar69_test.go` prints nothing: TestAR80_7 and every pause test green unchanged |
| budget | 60 turns, 180k context, `claude-opus-5-5`; stop at 48 turns or 144k (commit the act in flight, report); four acts, one commit each: 1 the tests, red; 2 the answer and the hold; 3 the stop, the kind, the plan's line; 4 the docs; charter `history/sittings/2026-10-30-4-ar98.md` |
| verify | `make check` |

## 1. What the source says (quoted)

"Relay ask: `five_hour`: today's map, on the current login. `none`: `denied` stops the night as a weekly limit does (`internal/night/pause.go:111`); the band never holds a row."

"Relay `Reset`: `five_hour`: the answer's `resume_at`, else the account's `resetsAt`, else the top level: one Max login reads today's reset. `none`: the answer's `resume_at`: the week's reset."

"Night pause: unchanged: a five_hour refusal pauses every row (relay entry 12). `none`: unchanged; a weekly refusal stops the night."

"Stream kinds: gains `seven_day_overage_included` as model `fable` (mode-free): only Fable rows pause."

"`status`, the relay's plan: `five_hour` unchanged; `none`: the band and its end, from band.json."

"With two or more logins a row's `wait` carries no `resume_at`, and band.json's top level no `resetsAt`, so today's `open()` errs and the row spawns `unchecked`; AR98 holds a `wait` that has no reset, or the one-spawn-at-a-time gate holds only in the verdict."

The record's tests: `TestAR98_1_NoneDeniedStopsAsWeekly`; `TestAR98_2_ResetReadsAnswerResumeAt`; `TestAR98_3_FableTypeIsModelFable`; `TestAR98_4_PlanShowsBand`; `TestAR98_5_WaitWithoutResetHolds`.

This repo's entries, verbatim:

> 15. **The gate, and what a red row does.** A child whose result is an error is red. Otherwise the row is green only when three checks hold: the child made a commit, its tree is clean, and the verify line is green or exits as its baseline on main did. The verify line is the repo's own `make check` by default, cut at entry 10's limit. A line already red on main is compared, never failed on its own. A gate that could not run is unrun, and unrun is red: unknown is never a pass. A red row is parked with its reason, never retried the same day; a new attempt is a new row with a new id. A gate red moves the night on; a sitting failure stops it. An expired login stops the whole run, not one row: the child's uncommitted files stay in its tree, and the next attempt is free. A usage limit pauses the run and no longer stops it: the turn it ended stands as it is, the child's uncommitted files stay in its tree, a wake armed across the plan's reset re-enters the loop, and the sitting reads its charter's act in flight before it starts anything, so no act is duplicated; agentik-session's launcher, `agentik session run`, restarts a sitting the same way. One model's own limit pauses only that model's rows (entry 12). (2026-09-22 and 2026-09-23, two entries merged 2026-09-29; portfolio entry 23 until 2026-10-17, moved under agentik-hq entry 4; amended 2026-10-11, the owner's word of 2026-10-10 21:06: a usage limit pauses the run; amended 2026-10-12 by entry 12)

> 12. **When one model's limit is hit, should only that model's rows pause.** When one model's own limit is hit and names its reset, only that model's rows pause; rows on other models keep running, and the paused rows continue at the reset (packet AR71). The five-hour window's limit still pauses the whole night, as the owner ruled on 10-10. Entry 15 carries this sentence, by citation. (2026-10-12; the owner's answer to decision 12, the owner's word in the observer session, 2026-10-12, entered 03:18)

## 2. Rulings

1. **The answer.** `BandVerdict` gains `Window string` (`window`) and `ResumeAt string` (`resume_at`); either absent or null reads empty, so an older plugin's answer reads as today. Verdict lines keep their keys.
2. **The stop.** `denied` with `Window == "none"` is a new `bandEnd`, `bandWeekly`, checked before `bandHolds`, on the first ask and on any ask inside a hold: the row is not spawned, the hold's `ends` line closes, the night reads `stopped:usage-limit` with detail `a weekly limit`, as TestAR65_4's `weekly` case reads it, with no wake line and no pause counted. Use the stop the weekly path already takes; a parallel path that spells the state again is a block. `denied` with any other window, or none, holds as today: `bandHolds` is unedited.
3. **The reset.** `open` takes the verdict's answer: a `ResumeAt` that parses RFC 3339 and is after now is the hold's reset; else `fx.Band.Reset()` as today; an unparsable non-empty `ResumeAt` falls to `Reset()`. A relay ask is judged on the current login and band.json's top level is the current login's, so `Reset()` keeps its signature and no account lookup is added. `OSBand.Reset` is otherwise unchanged.
4. **A `wait` with no reset.** Only for verdict `wait`, when `ResumeAt` is empty and `Reset()` errs: the hold opens with a zero reset rather than `unchecked`. The loop's reset case and the wake list skip a zero reset, so the row re-asks each `BandPoll` until the verdict moves, `--until` or the context. The held line reads as a hold's does, with no reset. A row queued behind a head holding this way holds too, rather than going `unchecked` on `Reset()`'s error; a row queued behind any other head, and every other verdict with `Reset()` erring, stays `unchecked` (TestAR80_7).
5. **The kind.** `eventKinds` gains `"seven_day_overage_included": {KindModel, "fable"}`.
6. **The plan's line.** `plan` reads band.json's top level, the file `OSBand.Reset` finds, and never asks. With `window` not `none`, or `band: off`, or no readable file, its output is byte for byte today's. With `none`: one line, `band: no 5h window · avoid to 11:00` when `demand` is there (`demand.band`, and HH:MM from `demand.until`'s own digits), `band: no 5h window` when not; the JSON form gains `band` with `window`, `demand` only on `none`. S44 writes `demand` as `{"band": "avoid", "until": "<zoned ISO>", "next": {...}}`; this packet reads no more of it. Pin both forms in `TestAR98_4`.
7. **Status.** The relay's `status` is not touched here: the plan's line alone. A filing for `NICE_TO_HAVE.md`.
8. **No bump.** Nothing under `plugin/`, so no plugin version.

## 3. Interfaces (signatures, not prose)

`effects.BandVerdict{Verdict, Mode, Text, Peak, Window, ResumeAt}`; `effects.Band` unchanged; `night.bandEnd` gains `bandWeekly`.

## 4. Findings that name this row

None.

## 5. Forbidden reads

Transcripts; `~/.local/state`, every part; every other repo, but `GUIDE.md`; band-pacing-r1 beyond §2, by section.

## 6. Report (under 15 lines)

Commits; tests, with the acceptance counts; how the stop reaches the night, by file and line; forks; anything unmet.
