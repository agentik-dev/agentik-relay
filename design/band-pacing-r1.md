# agent-relay design: a night paces its spawns by the session band (r1)

**Status:** design r1, 2026-10-14, written before code and revised once after its one refuter round (§7), which blocked on F1, now fixed. It amends relay-r1 §9 (configuration) and §15 (the breaker) and consumes the session plugin's admission verdict, never recomputing it; the plugin keeps `-p` sessions in `observe` "until agent-relay checks rows itself" (admission-r1 §2.4). The owner's word (agentik-session `OPEN-QUESTIONS.md`, answers 2, 4, 5): helpers run to 97% of each five-hour window; nights and sittings share it, the first to start taking the room; an earlier weekly reset is the band's horizon.

## 1. The problem

A night picks rows up to `--parallel` and reads no usage (`PickIn`, `internal/night/night.go:1634`), meeting the account only at a limit (usage-limit-resume r1 §2). Night 2026-10-14-2 opened at 06:15 with `--parallel 2`, spawning two rows 30 s apart. Replayed through the plugin's `decide` (`admission.py:776` at 9333ff9) over its ledger then, the first read `admitted` (peak 90%), the second `not-started` (115%), and each two-minute sample from 06:18 to 06:50 room 0 (143% to 217%). At 06:48 the live band read 38% used, 141% projected.

## 2. The decision

1. **Present or absent.** The band is present when `agentik-session` resolves at the night's open (`resolveTool`, `pin.go:472`), outside `PinTools`: a pinned tool that moves unruns every later gate (`gate.go:237`), and a reinstall mid-night must not. The config's new `band` key is `pace` (default) or `off`; absent or off, nothing is asked and every spawn runs as today (agentik-commons GUIDE §3).
2. **One ask, at a row's first spawn.** In `RunRow`, ahead of the breaker (`run.go:188`), a row's first part or orchestrator sitting runs `agentik-session admission admit --session <id>`, stdin `{"model": <its --model>, "subagentType": "relay-row", "background": true, "fork": false, "tool_use_id": "<night>/<row>/<part>"}`, timeout 10 s; the plugin decides in its lock and records a `verdict` line; the relay writes none of its files. No later spawn asks: a continuation, entry 11's fallback part, entry 12's part at the reset, or a part resumed under them. `<id>` is the part's own `--session-id` (`plan.go:414`), fresh, with no readings, so the plugin projects a new session, and the verdict keys to the session that runs. §1's samples asked so (143-217%); under the id the row ran with from 06:16, counted as running, they read 119-179%.
3. **The verdict, read.** `admitted`, `unread`, `stale` spawn; `not-started`, `deferred`, `wait`, `denied` hold. Anything else (non-zero exit, timeout, output not one object, unknown verdict) is `unchecked` and spawns, as the plugin's Mod passes a failed hook (admission-r1 §3.3). The `mode` is recorded, not obeyed: obeying `observe` would never pace.
4. **The hold.** A held row keeps its slot and stage and asks again every `night.BandPoll`, 60 s through the clock, as the plugin's wake (admission-r1 §3.4). Held rows queue; only the first asks, the next one poll after it spawns. A hold never fails a row and is bounded: at the five-hour reset, `band.json`'s `resetsAt` (still that reset since agentik-session a5a3656), the row spawns unasked, as a paused row continues at the reset; no `resetsAt` spawns it `unchecked`. Else it ends as the breaker's wait (`run.go:203-215`): `stop` and `--until` skip the row (`owner-stop`, `until-reached`, tree kept), `kill` or a signal fails it `owner-kill`, a sibling's limit hit `usage-limit`, paused as usage-limit-resume rules. `denied` (the week at its ceiling) sends one notice, quiet hours kept, to move accounts (answer 2), the usage-limit pause its backstop after the reset. The sitting's clock (`run.go:186`) starts after the hold.
5. **With entries 11 and 12.** Neither changes: a hold is not a pause, only a first spawn asks, so entry 11's fallback part still continues at once, and a hold ends at the reset where entry 12's paused row continues. The five-hour hit still pauses the whole night, a model's own hit only its rows.

**Rejected.** Deciding from `band.json`, display only (`admission.py:880-882`): it ages between writes, counts the last helper's family (`:927`), and would copy the verdict's rules into Go, session-layer-r1 §1's frozen layer; the hold reads only `resetsAt`, which does not age in its window. Stopping on `denied`: the owner would restart the night. An ask at every part: it changes entries 11 and 12.

## 3. The seam and the record

- `internal/effects`: a `Band` effect, `Ask(ctx, session, request)` and `Reset()`; `OSBand` runs the file and reads `band.json`, `FakeBand` scripts both; `band` is read beside `caffeinate` (`config.go:205`).
- `internal/night/run.go`: the wait; `RunOptions` gains `Until` and the held rows' queue (`:106`).
- The ledger: `night-open` gains `band` (the file, `absent` or `off`); a `band` line marks a hold's start (`state: held`, `verdict`, `peak`, `text`), end (`state: admitted` or `reset`, `asks`, `wait_s`) and `unchecked` (`error`), with `row` and `part`; `spawn` gains `band: {verdict, peak}`, `sitting` `band: {asks, wait_s, verdict}`; `json-keys.golden` is refrozen.
- `status` (`status.go:25`, `:47`): a held row reads `held` (display only), its reason the band's text; `--json` gives it `band: {verdict, peak, since, asks}`, and `next` leads with `the band holds <rows> since HH:MM`. `report` counts holds and their wait. `doctor` shows the file and its `--version`, `absent` or `off`, never asking (an ask writes a verdict line).

## 4. The proof

**P1, the census, 2026-10-14 06:55.** A read-only script replayed every relay `spawn` since 2026-10-12 10:41, the plugin ledger's start, through `decide` over the ledger to that second, with §2.2's request: 36 spawns in 10 five-hour windows, 27 `admitted`, 9 `not-started`. The two windows that reached 100% held five of the nine and no admitted spawn: the relay's two window pauses, 10-12 11:12 and 10-13 23:26, each after a spawn at 99-100%. No window whose relay spawns all read `admitted` passed 88%. Two ended under the ceiling with a `not-started`, at 60% and 79%; re-asked each minute, their holds would have lasted 101 to 117 and 12 minutes. Upper bounds: the replay keeps the relay's own burn.

**P2, the cost.** `agentik-session --version` takes 0.05 s; one `decide` over 4,083 ledger rows, 0.08 s.

**P3, the weekly horizon.** Today the week resets at 08:00, the window at 10:30: the band's horizon is 08:00, `resetsAt` 10:30. The relay reads no horizon; its input changes, its code does not.

## 5. The cut into packets

1. **AR80, the ask and the hold.** §2 and §3, by named tests over `FakeBand` and the step clock: absent or `off` asks nothing; `admitted`, `unread`, `stale` spawn at once; `not-started` twice then `admitted` spawns after two polls (two `band` lines, `asks: 3, wait_s: 120`); of two held rows only the first asks; `stop`, `kill` and `--until` end a hold within one poll, the reset with an unasked spawn; exit 3, a timeout, bad JSON, an unknown verdict or no `resetsAt` spawn `unchecked`; `denied` holds with one notice; a continuation, a fallback part and a part at the reset never ask; a row held before its first spawn and paused by a sibling's limit hit fails `usage-limit`, then spawns at the wake unasked.
2. **AR81, the read.** §3's `status`, `--json`, `report` and `doctor`, by named tests over a held night's ledger.

## 6. Open risks

- **The projection may over-read.** P1's 60% and 79% windows ended under the ceiling with a hold in them; calibration is the plugin's (admission-r1 P1 to P3), the owner's lever `band: off`. 10-14's projection was the real burn, no over-read: 31% at 06:44, 58% at 07:00.
- **A row is projected as a helper** (a 15-minute seed life, admission-r1 §3.2; 10-14's rows ran 8 to 36 minutes) under the default ceiling: filed with the plugin, the usage-limit pause catching an under-read.
- **A relay hold is never counted.** An ask records `not-started`, outside the plugin's `HELD` (`deferred`, `wait`), so `band.json`'s held count and `report` never show one, and the plugin cannot ask without recording: a session-plugin follow-up.
- **Night.tla** checks row progress under `LoadFair` (a load the breaker waits on falls); the reset gives a hold that guarantee. No model is needed: a hold is one more guard on `Spawn` that the reset lifts, adding no state, stop or process; a packet models it if a hold gains another end.
- **Fail open.** A broken plugin turns pacing off, seen only in `unchecked` lines and doctor; the plugin fails open too (a lock wait over 2 s exits 3, which spawns), its doctor checking only `--version`.

## 7. The refuter round (2026-10-14)

One adversary, read-only, at 049bc2c: BLOCK on F1, notes N1 to N6; the proof holds. F1: every part asked (§2.2) and only `--until` and `stop` bounded a hold (§2.4), so entries 11 and 12 changed though §2.5 said not; now only a first spawn asks and a hold ends at the five-hour reset. Taken too: the session id (§2.2, re-run under both ids); N1, N2, N3, N5 (§6); N6 (§5); renumbered AR80, AR81 (AR76, AR77 taken). N4 confirms P3 (band-target reaches the relay through the verdict).
