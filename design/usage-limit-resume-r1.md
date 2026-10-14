# agent-relay design: the usage limit pauses the night (r1)

**Status:** design r1, 2026-10-10, written before code; refuter round 1 folded. The owner's word, as the observer recorded it (example-mac `history/observer/2026-10-10.md:52`): "sessions and relay runs resume by themselves after a usage limit (the 5-hour window), instead of stopping." That window pauses. So do a model's own limit that names its reset (only that model's rows) and a limit text the relay does not recognize (one probe, 30 minutes on). A weekly limit stops, as today. This amends relay-r1 §1, §5 and §9; this repo's `DECISIONS.md` is silent.

## 1. The problem

`stream.Account` (`internal/stream/account.go:36`) names `usage-limit`; `RunRow` kills the child on a refused `rate_limit_event` (`internal/night/run.go:340`), no gate (`:478`); `collect` kills every other running row (`internal/night/night.go:1014-1024`). The night ends `stopped:usage-limit`, every row skipped. The ledger holds five (PDT). Two are weekly: es2a (fable, 10-02 10:36, `resets 2pm (America/Los_Angeles)`) and mutdiff (opus, 10-09 00:38, 14:00, 13 h 21 m on). Three are the Fable limit, no reset: rtfix3 (10-05), ws5y (10-07 19:57), fen3c (10-08 05:41, turn 1, $0, 9 h 43 m after ws5y). Each `exit` line reads `child_exit` 143, `is_error` true. Neither text matches a needle (`account.go:22-25`) or reads `API Error: NNN` (`:29`), so the event named all five. That is inferred: the relay keeps no raw stream.

**The CLI's own records.** The `isApiErrorMessage` lines of `~/.claude/projects/**/*.jsonl` (96 files with subagents, CLI 2.1.263 to 2.1.296, grepped 2026-10-10) hold three kinds; `…` is `(America/Los_Angeles)`:

| kind | the text | hits | `quotaLimits` |
|---|---|---|---|
| session | `You've hit your session limit · resets 1:30am …`; `6:20pm`, `10pm` | 35: 22 `N:Nam`, 8 `N:Npm`, 5 `Npm` | `five_hour` |
| weekly | `You've hit your weekly limit · resets Sep 28 at 4pm …`; `2pm` | 35: 29 `Mon D at Npm`, 6 `Npm` | `seven_day` |
| model | `You've reached your Fable limit. Run /usage-credits … or switch models with /model.` (cli, 39); `… Switch to another model, or manage usage credits …` (sdk-cli, 7) | 46 | none |

None names a weekday or the form `Oct 9, 2pm`. es2a's `resetsAt` is 1790974800 (Fri 10-02 14:00 PDT), mutdiff's 1791579600 (Fri 10-09 14:00 PDT). The 5-hour window, the owner's case, has 35 hits there and none in the relay's ledger. The one stream event recorded (`internal/stream/testdata/real.jsonl:8`) is an `allowed_warning`. Under §2 all five ledger hits still stop.

## 2. The decision

**Recognition** (the first account event wins, `internal/stream/stream.go:182`):

| the stream says | reason | reset |
|---|---|---|
| `rate_limit_event`, `rate_limit_info.status` not `allowed…` | `usage-limit` | `resetsAt` (Unix seconds), `rateLimitType` |
| an `is_error` result (not `error_max…`): `API Error: 429` or a usage needle | `usage-limit` | the text's clause, when the event gave none |
| the same: `API Error: 401` or an auth needle | `auth`: stops the night, as today | none |
| anything else | today's (`stream-error` …) | none |

**The kind.** The needles gain `hit your <word> limit` and `reached your <word> limit` (`<word>` one word). The kind is the event's `rateLimitType`, else the text's: `five_hour` or `session`, the **window**; `seven_day` or `weekly`, **weekly**; `reached your <word> limit`, a **model's** (both Fable texts); else **unknown**.

**The reset** reads only the measured forms, `resets <when> (<zone>)`: a time (`1:30am`, `6:20pm`, `2pm`) or a date and time (`Sep 28 at 4pm`), the first such after the hit. The zone is an IANA name (`time.LoadLocation`, with `time/tzdata`); absent, the local one. The old `…limit reached|N` text is only this repo's test fixtures (57 of the 58 transcript lines that hold it), never the CLI's, so it reads as no reset. Any other text, an unknown zone, or a reset not after the hit or past 8 days: no reset. The wake is the reset plus 2 minutes.

**The pause.** One pure function, `pauseFor`, decides at the hit. It stops the night `usage-limit` at once: on a weekly limit; on a model's limit with no reset; when the wake is at or past `--until`; with no `--until`, when it is over 6 h away; after 12 pauses; when a wake's sitting is refused with a known reset not after the one waited out; or while draining. None of these probes for hours. An unknown limit with no reset waits 30 minutes (source `fallback`), and that wake's sitting is the probe (fen3c's refusal took 1.1 s, $0); a probe refused again with no reset stops the night. Else the night steps `OnLimited` to `paused:usage-limit`. A window or unknown limit kills every other running row as today, and each comes back paused; a model's limit pauses only that model's rows. A later reset among them moves the wake later. A sibling killed in its gate (`night.go:794-800` keeps it in `running`) is gated again at the wake, never given a child. A `pause` line holds the rows, `resets_at`, `source` (`event`, `text`, `fallback`), the kind, the wake and the count; its notice keeps quiet hours. Each paused row keeps its `Staged` and its stage lock, and `PickIn` (`internal/night/schedule.go:63`) skips it.

**While paused** the loop picks no row of the paused scope. Its poll (`opts.Poll`, `night.go:1160`) compares the wall clock (`fx.Clock.Now().Round(0)`) with the wake: no timer. `relay stop` reads `stopped:owner-stop` at the next poll; `relay kill` or a signal, `stopped:owner-kill`; `lock-moved` as today. A night that ends paused halts each paused row `failed: usage-limit` (`Step(Paused, Halt)`), unlocks it, keeps its tree (`AccountKept`, `internal/night/stage.go:531`) and prints its `relay resume` line.

**The sitting line.** A limited part's `sitting` line stays `failed: usage-limit` and gains `limit: paused` when the night pauses on it. `redSittings` (`cmd/relay/main.go:629-634`), `nextSitting`'s red list (`night.go:824-829`) and `orc.stuck` skip that line, so a recovered night parks nothing (`main.go:604-606`). A night that then stops names the row in its `stop` line's resume lines, which park it as today.

**The wake.** The night steps `OnWake`, writes a `wake` line and checks `--until` and `--max-usd` (`night.go:1183-1188`), not `--max-sittings` (AR58 ruling 3). A failed check stops the night and halts the paused rows. Else each paused row, in queue order, gets a **continuation**:

- The tree check: the worktree is on the row's branch and its HEAD descends from the stage's `head_before`; else the night stops `usage-limit`, `<row>: the tree moved while paused`.
- A new session id; `--max-turns` and `--max-budget-usd` are the row's less its parts' tonight, never under 10 turns or $5. The context ceiling and the wall are per sitting.
- The start hook records `head_before`, not the new HEAD (`internal/hooks/start.go:33-38`), so a part with nothing left is not nudged for `no-commit` (`stop.go:47-53`).
- The prompt: the row's own and a RESUME block naming the part, the last session, the commits since `head_before` (at most 20) and the uncommitted paths (at most 40): these files are yours; read them, keep what is sound, continue from the first act your commits do not cover.
- `RunRow` on the first stage's `Staged`, so the gate judges every part against one cut; `part` on its lines.
- An orchestrator sitting is not continued; its next sitting is cut as orchestrator r1 §4 cuts any.

A continuation is not a pick: `picked` (`night.go:1208`) does not read it as no-progress, an exception to relay-r1 §1's "a row attempted twice in one night".

**The lock and the gate.** The relay lives through the pause, so `relay.pid` stays held (§6). The stage locks stay held, and `clean --merged` keeps a tree whose stage lock has a live holder: four of five limited rows ended at `head_before`, an ancestor of main (`main.go:973-975`), so a clean tree would go (`internal/effects/git.go:464-466`). A limited part runs no gate (`run.go:478`).

**Entry 23, proposed** for example-ops's ledger, which this record does not edit. It carves three phrases for the pause: "a child whose result is an error is red" (all five had `is_error` true), "never retried the same day" and "a new row with a new id". Proposed: "A child stopped by the 5-hour window's usage limit is not red, is not parked, and keeps its id: the run pauses until the window resets, then continues the same row the same day in its own tree. A model's own limit that names its reset pauses only that model's rows the same way, and a limit the relay does not recognize pauses the run once, for 30 minutes. A weekly limit, a model's limit with no reset time, a second unrecognized limit, a reset past the run's end, or an expired login stops the whole run, not one row: the child's uncommitted files stay in its tree, and the next attempt is free."

## 3. The seam

- `internal/stream/account.go:17-26`, `:36-47`: the needles, the kind, the clause reader, a `Limit` (reason, reset, source, kind, scope) on `Tally`; `testdata/account-cells.tsv` and a new `reset-cells.tsv` of the measured texts.
- `internal/relay/machine.go:5-12`, `types.go:220-224`, `:263-278`, `night.go:22-30`: `Paused` (`Limited` from `Exited`; `Spawn`, `Halt`), `paused:usage-limit` (not terminal), `OnLimited`, `OnWake`.
- `internal/night/run.go:505-509`: `limit` on the `sitting` and `exit` lines. `night.go`: `pauseFor`, the deadline (`:1178`), the continuation beside `start` (`:721`), no unlock at `:796` for a paused part, the halt (`:1287`). `plan.go:933-944`: the argv. `status.go:24-41`, `:212-246`: `wake`, `next` led by `wakes at 14:02 (resets 14:00, event)`, the row `paused`.
- `internal/prompt/template.go:28`, `:75`: the RESUME block. `internal/effects/fake.go:565`: `FakeSpawner` gains `Outs`, one stream per `Start`.
- The tests the pause breaks: `internal/night/ar17d_test.go:54`, `:61` (the night stops) and `cmd/relay/e2e_test.go:808` (a past Unix time: 30 minutes against a 1-minute bound).

## 4. The proof

No probe ran; the evidence is §1. Each test runs a night on `FakeSpawner` (spawn 1 a rejected `five_hour` event, `resetsAt` T+1h, exit 143; spawn 2 green) and `StepClock` (`internal/effects/clock.go:17`), in the packet named.

1. **Pause, wake.** Packet 3: `paused:usage-limit`, `wake` T+1h+2m, one `pause` line, `limit: paused`; the clock still, no spawn, no line per poll; past the wake the row halts. Packet 4: a `wake` line, spawn 2 in the same tree, a new session, the turns' remainder, a RESUME block naming `FakeGit.Status`'s file; the gate on the first cut; `queue-empty`; a moved tree stops.
2. **The stops** (packet 3). A weekly limit, a model's limit with no reset, a reset past `--until`: `stopped:usage-limit` at once, no `pause` line. `--max-usd` at the wake: `budget-reached`. A 401: `stopped:auth`.
3. **The owner** (packet 3). `stop`: `owner-stop` in one poll; `kill r1`, `kill *`, SIGINT: `owner-kill`; the row `failed: usage-limit`, unlocked, its tree kept.
4. **The guards** (packet 5). The thirteenth pause and the same reset twice stop with each detail. At `--parallel 2` one window hit pauses both rows; both continue, spawned apart. A model's limit with a reset pauses only its rows. A sibling killed in its gate is gated again.
5. **The texts** (packet 1). Each measured form reads its kind and reset: `1:30am`; `6:20pm`; `2pm (America/Los_Angeles)` at 10:36 is 14:00 that day, at 15:00 the next; `Sep 28 at 4pm` that date; both Fable texts a model's limit, no reset; `limit reached|1758000000` no reset (a fixture's form, never the CLI's); an unknown zone none.

## 5. The cut into packets

Numbered at the cut; each fits one sitting, and its tests need no later packet.

1. **The facts** (`internal/stream`, the `exit` line's `limit`): test 5. Nothing pauses yet.
2. **The machines** (`internal/relay`): the states, the events, the matrix tests; relay-r1 §1.
3. **The pause** (`internal/night`, `cmd/relay`): `pauseFor`'s stops, the `pause` line, `limit: paused`, the deadline, the owner, `PickIn`, the halt at the wake, `redSittings`, `clean --merged`; the three tests it breaks (§3); tests 1 (to the halt), 2, 3; relay-r1 §5, and §1's `AR17D act 0` re-pointed.
4. **The wake** (`internal/night`, `internal/prompt`, `internal/hooks`, `Outs`): the continuation, the tree check, the start hook, `picked`; test 1's wake.
5. **The guards**, after 4: the pause count, the same reset, a model's scope, the sibling gated again; test 4.
6. **The readers**, after AR60: status, report, the stop verb's line, the attendant, relay-r1 §9 and §11.

## 6. Open risks

- **A model's limit stops the night.** All 46 measured named no reset; fen3c's outlasted 9 h 43 m. The owner's question 1.
- **A long pause meets the owner's day.** The held `relay.pid` refuses `gate`, `plan`, `run` and `resume` (`main.go:446`, `:793`, `:1163`, `:1592`) for up to 6 h, or to `--until`. A push to main while paused reads `origin-moved` or `main-moved` at the gate.
- **A sleeping Mac** wakes the night late, never early (`caffeinate` wraps only spawns, `plan.go:937`).
- **The stream's fields are the CLI's**, published since in the Agent SDK references (amended 2026-10-14, research fixes). The Python page's `RateLimitInfo` (code.claude.com, as of 2026-10-14) has `rate_limit_type`, one of `five_hour`, `seven_day`, `seven_day_opus`, `seven_day_sonnet` and `overage`, and `resets_at`, and keeps the CLI's own dict in `raw`; the TypeScript page spells `resetsAt` (per the research site's reading of that page, 2026-10-14). The wire spelling the relay reads, `rateLimitType` and `resetsAt` under `rate_limit_info`, is the one recorded in `internal/stream/testdata/real.jsonl:8`; the snake case is the Python SDK's. `eventKinds` maps `five_hour` and `seven_day` only, so a model family's weekly limit (`seven_day_opus`, `seven_day_sonnet`) and `overage` have no kind of their own. The spelling's re-test on the current CLI and the two per-family values are filed in `NICE_TO_HAVE.md` (2026-10-14).
- **The remainder's source.** `status.go` buildStatus holds a part with no result (a killed sibling) at the row's full usd (NICE_TO_HAVE:251): read from there, each killed sibling floors at $5 and `--max-usd` can trip early. Turns differ by line too (es2a: `exit` 44, `sitting` 38). Packet 4 names the source.
- **`Night.tla`** models the account stop as a stop (`design/tla/Night.tla:417-428`, `NoSpawnAfterAccountStop` at `:806-807`); it diverges from Go once packet 3 lands.
- **A lock left mid-commit.** A sibling killed mid-commit may leave `index.lock`, which the tree check does not read. Observed, not reproduced.

## 7. Choices for the refuter

Ruled in round 1. Kept: the 2-minute margin, the 30-minute wait for an unknown limit, 12 pauses, 6 h with no `--until`, 8 days, the event's reset over the text's, the local zone, a fresh session over `claude --resume` (the cache is cold; es2a sat at 206k context), continuations outside `--max-sittings`, the floors, the full wall per part, no draining while paused, a deadline on the poll, a kill of a paused row stopping the night, an orchestrator sitting re-cut.

Changed:

- The limited part's `sitting` line gains `limit: paused`, or `redSittings` parks a recovered row for the day.
- A model's limit with a reset pauses only that model's rows.
- A model's limit with no reset stops the night, never waiting 30 minutes.
- `clean --merged`'s guard lands with the pause (packet 3), since a clean limited tree is removable from the first pause.

## 8. Choices for the owner

1. **A second model.** A repo's config may name `limit_fallback_model`, off when unset; when set, a model's limit continues the row at once on it. Without it, every measured model limit stops the night. Recommended: build it, as packet 7.
2. **No `--until`.** Such a night waits at most 6 h, the 5-hour window and a margin; a weekly limit stops it at once. Recommended: as written.
