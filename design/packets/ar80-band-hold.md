# Packet AR80: a night asks the session band at a row's first spawn and holds while it says no room

| item | value |
|---|---|
| from | `design/band-pacing-r1.md` (c63f5d3) §2, §3 (but `status`, `report`, `doctor`), §5 packet 1; `DECISIONS.md` entries 11, 12; agentik-session at e8477a7; the code at 7450c05 |
| pin | agent-relay `main` at 7450c05 (main plus the AR75 to AR77 merges) plus this packet's own commits; a linked worktree, branch `ar80-band-hold` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness; never run `agentik-session`. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet` and `gofmt -l` of the three packages, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/effects/band.go`, `internal/effects/ar80_test.go` (new), `internal/effects/effects.go` (`Effects.Band`), `internal/effects/config.go` (the `band` key), `internal/effects/notify.go` (the `band` notice), `internal/night/run.go` (`RunOptions`, `BandPoll`, the queue, `RunRow`, the `sitting` line), `internal/night/night.go` (`NightOptions.Band`, `night-open`, `launch`), `internal/night/ar80_test.go` (new), `cmd/agentik-relay/main.go` (`bandFor`, the run verb), `cmd/agentik-relay/main_test.go` (`TestMain`), `design/relay-r1.md` (§5, §9, §11), `design/band-pacing-r1.md` (Status), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/effects -run '^TestAR80_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR80_'` reads 1; `go test ./internal/night -run '^TestAR80_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR80_'` reads 13; `go test ./internal/effects -run '^Test(LoadConfig\|Notices\|AR75_\|AR80_)' -count=1 2>&1 \| grep -c -E '^ok '` reads 1; `go test ./internal/night -run '^Test(AR13_5_\|AR65_\|AR67_\|AR71_\|AR77_\|AR80_)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 |
| budget | 90 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

band-pacing-r1 §2.2: "a row's first part or orchestrator sitting runs `agentik-session admission admit --session <id>`, stdin `{"model": <its --model>, "subagentType": "relay-row", "background": true, "fork": false, "tool_use_id": "<night>/<row>/<part>"}`, timeout 10 s … No later spawn asks". §2.3: "The `mode` is recorded, not obeyed". §2.4: "asks again every `night.BandPoll`, 60 s … only the first asks, the next one poll after it spawns … at the five-hour reset, `band.json`'s `resetsAt` …, the row spawns unasked …; no `resetsAt` spawns it `unchecked`. Else it ends as the breaker's wait … `denied` … sends one notice, quiet hours kept, to move accounts". §3: "a `band` line marks a hold's start (`state: held`, `verdict`, `peak`, `text`), end (`state: admitted` or `reset`, `asks`, `wait_s`) and `unchecked` (`error`), with `row` and `part`".

The plugin: `admit` answers `{verdict, mode, text, peak}`, `peak` null for `unread`, `stale` (`claude_session/admission.py:897`); `band.json` is `<XDG state>/agentik-session/admission/band.json`, old folder `claude-code-session` (`config.py:13-15`, `admission.py:904`), `resetsAt` zoned ISO (`:960`); a lock wait past 2 s exits 3 (`:280`).

The code (cites moved): `resolveTool` `internal/effects/pin.go:474`; `caffeinate` `config.go:324`; `RunOptions` `internal/night/run.go:70`, `spacer` `:106`; `launch` `night.go:762`, `continueRow` `:873`; `night.Run` `cmd/agentik-relay/main.go:1347`. Entry 11's fallback is unbuilt.

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar80.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays (§2.5). The golden stays (AR81's).

1. **Act 1, the effect.** `OSBand.Ask` runs the file, the request on stdin, under `BandTimeout`; a non-zero exit, the timeout or stdout not one JSON object errs. `Reset` reads `band.json` under `xdg.StateHome`, new folder else old; none, no `resetsAt` or not RFC 3339 errs. `ResolveBand` gives nil and `off`, nil and `absent`, or an `OSBand` and its `resolveTool` file. `FakeBand` scripts each row's answers (the last repeating) and `Reset`, and records asks. The `band` key, beside `caffeinate`: `pace` (default) or `off`, else an error naming its line.
2. **The wiring.** The run verb sets `fx.Band, no.Band = bandFor(pd.cfg.Band, getenv)`; `effectsFor` stays bandless; `TestMain`'s `bandFor` gives nil, `absent`. `NightOptions.Band` fills `night-open`'s `band` ("": `absent`).
3. **Act 2, the ask**, in `RunRow` before the breaker, iff `fx.Band != nil` and `opts.Part < 2`; `tool_use_id` `<RunOptions.Night>/<rp.ID>/1`. An error or a verdict outside §2.3's seven is `unchecked`, its `error` named; so is a holding verdict whose one `Reset()` errs or is not after now. An ask the context cuts ends as ruling 5. No `mode` is written: the plugin's line holds it.
4. **The hold** writes its start line and waits on the clock for the nearest of the next `BandPoll`, the reset and `RunOptions.Until`, or the context. A re-ask writes nothing but a hold's first `denied` that did not open it: one more `held` line. A spawning verdict ends it `admitted`, the reset `reset`, an `unchecked` re-ask its line with `asks`, `wait_s`. `start` (`run.go:186`) follows the hold.
5. **Its other ends** take `run.go:203-214`'s branch; `Until` reached skips `until-reached`; each through `sit.end`, tree kept, no end line.
6. **The queue** (unexported in `RunOptions`, set at `launch` as `space`): rows take places in launch order; only the head asks. One behind a held head writes its held line: `verdict: queued`, `peak` null, `text` `behind <head>`, its own `Reset()` read. The next asks at once after an unheld head, one `BandPoll` after a held one, however it left. At the reset all spawn, spaced as today.
7. **The lines.** `part` is 1. `spawn.band.verdict` is the plugin's, `reset` or `unchecked`; an unheld part's `wait_s` is 0; a part that never asked has no `band` key. `Notices` maps a `band` line only at `held`, `denied`: `verdict`, `peak`, priority 3, the body ending `; move accounts`.
8. **Tests.** `TestAR80_1_OSBand` (effects; a script as the file): an answer, argv and stdin; exit 3, a 200 ms `Timeout`, `not json`, two objects; `Reset`'s time, old folder, errors; the key; `ResolveBand`. Night, `ar65World` with a `FakeBand`, T its start, resets T+2h: `TestAR80_2_AbsentOrOff` (`night-open` says which; no `band` key). `TestAR80_3_SpawnsAtOnce` (each spawning verdict: spawn at T, no `band` line, `sitting.band` `{1, 0, v}`). `TestAR80_4_HoldThenAdmit` (`not-started` twice: asks at T, +60 s, +120 s; two lines, `asks: 3, wait_s: 120`; spawn and wall from +120 s). `TestAR80_5_QueueOneAsks` (`--parallel 2`, r1 as _4: r2 `queued`, its first ask at +180 s). `TestAR80_6_HoldEnds` (stop, kill, `--until` +90 s: within a poll, no spawn, tree kept; resets +150 s: `reset`, spawn then). `TestAR80_7_Unchecked` (three errors, verdict `maybe`, a failing `Reset`: an `unchecked` line each, spawn at T). `TestAR80_8_DeniedNotice` (`denied` twice: one notice; `not-started`, `denied`: two `held` lines, one notice). `TestAR80_9_PartsNeverAsk` (r1 refused by a `five_hour` event, resets T+1h: one ask, part 2 at the wake; `RunRow` with `Part: 2`; `ar71World`'s part at its reset). `TestAR80_10_HeldThenPaused` (`--parallel 2`, r1 as _9, r2 `not-started`: r2 `failed`, `usage-limit`, `limit: paused`; part 2 at the wake, unasked).
9. **Act 3, the record.** relay-r1 §5: the line, the notice; §9: the key; §11: the tests. band-pacing-r1's Status: "AR80 built §5 packet 1".

## 3. Interfaces (signatures, not prose)

`effects`: `BandRequest{Model, SubagentType, ToolUseID string; Background, Fork bool}`; `BandVerdict{Verdict, Mode, Text string; Peak *float64}`; `Band{Ask(ctx, session, req) (BandVerdict, error); Reset() (time.Time, error)}`; `OSBand{File, Env, Timeout}`; `ResolveBand(mode, env) (Band, string)`; `FakeBand.Asks() []string`. `night`: `BandPoll`, `RunOptions.Until`, `.Night`, `NightOptions.Band`.

## 4. Findings that name this row

None in `NICE_TO_HAVE.md` (band, pacing, admission, band-target; grep at 7450c05).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state and agentik-session's. Read no other repo; section 1 quotes the plugin.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: each act's red line; each existing test changed; rulings beyond §2; anything filed. Last line: branch, head sha.
