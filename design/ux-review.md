# UX1: the printed surface at a6a2eba

Packet UX1 reviewed the human lines of `plan`, `run` (with the live line), `status`, `report`,
`doctor`, `resume` and `attach`. It checked each one against the packet's §2 checklist, in this order:

1. The first line says what state the night is in.
2. Every red or failed row prints `relay resume <row>` on a line of its own, and `status` and
   `run` end with those lines.
3. The report's headings come in the morning's order.
4. Numbers align in columns.
5. No home path, no JSON and no Go type name appears in a human line.
6. Every line is under 100 columns.
7. `usage` names `status` and `resume` first.

It changed wording and layout only. The `--json` key sets are pinned by `TestJSONShapesFrozen`
against `internal/night/testdata/json-keys.golden`, which was taken at a6a2eba before any edit
(status, report, plan, run, doctor). Exit codes and ledger lines are unchanged.

How the captures were made: the two-row night (r1 red, r2 green) over the fake claude is
`surfaces` in `internal/night/status_test.go`. The `cmd/relay` tests capture `run --dry`,
`doctor`, `resume`, `attach` and `--help`. Setting `UX1_CAPTURE=<dir>` writes each capture to
`<dir>/<verb>.txt`. The temp root reads `$REPO` and the state directory reads `$STATE`. The
base's `plan`, `doctor` and `--help` come from a6a2eba's binary, built from `git archive`
and run with a temp `XDG_STATE_HOME`. The machine lines replace home with `~`. The spawn
lines are cut at `...` here only.

`bin/relay status` at the base, with no night in the state directory, printed `no night yet`.
The first line names the night, so the acceptance's `grep -c night` reads 1. With a night it
printed `night <id>: <state>`.

## plan

Before (the fixture):

```
row r1: ready; branch r1 off main; worktree $REPO/.claude/worktrees/r1
  deps: satisfied none; pending none
  baseline on main: 1 lines (0 green, 1 red, 0 unrun) in 0.0s
    1. red: exit 0, last "no"; want reads `ok`
  spawn: cd $REPO/.claude/worktrees/r1 && claude -p --permission-mode ... < prompt
  resume: cd $REPO/.claude/worktrees/r1 && claude --resume 4217586c-e633-4707-9821-9e809c764e2c
row r2: ...
plan: night 2026-09-22-1; 2 rows; template sha256 7af0aa7a5c1b014e96fcd3f01b73c5fc972381602c8c3635a03a23c56db04312; stops queue-empty, owner-stop, origin-moved; valid
```

Checklist:
1. Failed: the verdict was the last line.
2. Not applicable, because a plan has no red row. Each row's resume door was printed.
3. Not applicable.
4. Not applicable.
5. Failed: the worktree and the resume door printed home whole on this machine (`/Users/...`).
6. Failed: the summary line ran 150 columns, and the spawn line runs over 200. On this machine
   19 of the real queue's plan lines were over 100.
7. Not applicable.

After:

```
plan: night 2026-09-22-1 would open with 2 rows; valid
  stops: queue-empty, owner-stop, origin-moved; template sha256 7af0aa7a5c1b
row r1: ready; branch r1 off main
  worktree $REPO/.claude/worktrees/r1
  deps: satisfied none; pending none
  baseline on main: 1 line (0 green, 1 red, 0 unrun) in 0.0s
    1. red: exit 0, last "no"; want reads `ok`
  spawn: cd $REPO/.claude/worktrees/r1 && claude -p --permission-mode ... < prompt
  resume: cd $REPO/.claude/worktrees/r1 && claude --resume 11cbf676-d751-4006-aaa8-0e5e189435b6
```

Fixed: the verdict comes first, the template hash is cut to 12 characters (the full hash stays
in `--json`), the worktree goes on its own line, the worktree and the resume door write home as
`~/` (a shell still expands that), `1 line` is now singular, and a baseline's last output is
clipped at 40 characters.

Not fixed: the `spawn:` line stays whole and over 100 columns. `TestPlanEqualsRun` in
`internal/night/night_test.go` (outside UX1's `may touch`) pins it as one line with the absolute
worktree. That is a handover item. `TestPrintedLinesUnder100` exempts it by name.

## run

Before, at the end of the night (the night's own lines, then the dashboard):

```
relay r1: running in $REPO/.claude/worktrees/r1
relay r1: red; relay resume r1
relay r2: running in $REPO/.claude/worktrees/r2
relay r2: green
night 2026-09-22-1: stopped:queue-empty
row  state  reason  turns  context   cost   elapsed  last tool  builders  merge
r1   red    -       3      71k/130k  $1.20  0s       Bash       1         -
r2   green  -       3      71k/130k  $1.20  0s       Bash       1         -
burn: 2 sittings, 6 turns, $2.40, 2 builders
resume: relay resume r1
```

`run --dry` printed `spawn: cd /abs/... < prompt`, then `prompt_hash <hex>` and
`rules_hash <hex>`. There was no line naming the night. A plan that was not valid printed as a
single line joining every problem (over 400 columns for the real queue here).

Checklist:
1. Failed for `--dry` (no night line). The tail passed through the dashboard.
2. Failed: the tail ended `resume: relay resume r1`, so the resume command was not a whole line.
3. Not applicable.
4. Failed: numbers were left-aligned.
5. Failed: the JSON key names `prompt_hash` and `rules_hash` were printed as words.
6. Failed: the invalid-plan error line.
7. Not applicable.

The live line (§7) was `turn N: <tool> "<summary>"; context 71k/130k; $1.20`. The summary is
capped at 80 bytes, so with the rest of the line it could reach about 120 columns.

After:

```
relay r1: running in $REPO/.claude/worktrees/r1
relay r1: red; relay resume r1
relay r2: running in $REPO/.claude/worktrees/r2
relay r2: green
night 2026-09-22-1: stopped:queue-empty; 2 rows: 1 red, 1 green
row  state  turns   context   cost  time  last tool  builders  merge
r1   red        3  71k/130k  $1.20    0s  Bash              1  -
r2   green      3  71k/130k  $1.20    0s  Bash              1  -
burn: 2 sittings, 6 turns, $2.40, 2 builders
to resume the red or failed rows:
relay resume r1
```

`run --dry`:

```
run --dry: night 2026-09-22-1, 1 row(s); nothing staged, nothing spawned
row a3-machine:
  spawn: cd /root/.claude/worktrees/a3-machine && caffeinate -i claud ... < prompt
  prompt sha256 9b6975465ae230f3d556a8b66e283d79a361873e8047ff30eb527e3e6943e5c6
  rules sha256  7af0aa7a5c1b014e96fcd3f01b73c5fc972381602c8c3635a03a23c56db04312
```

Fixed: `--dry` names the night first and writes the hashes as `prompt sha256` and
`rules sha256`. An invalid plan prints `relay run: the plan is not valid, N problems` and then
one problem per line. The tail's resume lines are bare and come last. `Tally.Line` cuts the
summary with `...` so the live line stays under `stream.LineCap` (100); `TestLiveLineUnderCap`
pins this.

Not fixed:
- The night's own line `relay r1: red; relay resume r1` still carries the resume command inside
  the line. `internal/night/night.go` is outside `may touch`, so this is a handover item.
- The live log repeats a line when one assistant message holds two tool uses (`turn 2: Edit`
  twice). That is the stream's event count, not wording, so it is a handover item.
- The `--dry` spawn line stays whole, to match plan's.

## status

Before:

```
night 2026-09-22-1: stopped:queue-empty
row  state  reason  turns  context   cost   elapsed  last tool  builders  merge
r1   red    -       3      71k/130k  $1.20  0s       Bash       1         -
r2   green  -       3      71k/130k  $1.20  0s       Bash       1         -
burn: 2 sittings, 6 turns, $2.40, 2 builders
resume: relay resume r1
```

Checklist:
1. Passed only partly: the first line gave the night's state but not its rows.
2. Failed: the resume command came as `resume: relay resume r1`, not as a whole line.
3. Not applicable.
4. Failed: numbers were left-aligned (tabwriter).
5. Passed.
6. At risk: ten columns plus `state` and `reason` side by side. In the fixture it was under 100.
7. Not applicable.

After:

```
night 2026-09-22-1: stopped:queue-empty; 2 rows: 1 red, 1 green
row  state  turns   context   cost  time  last tool  builders  merge
r1   red        3  71k/130k  $1.20    0s  Bash              1  -
r2   green      3  71k/130k  $1.20    0s  Bash              1  -
burn: 2 sittings, 6 turns, $2.40, 2 builders
to resume the red or failed rows:
relay resume r1
```

Fixed:
- The first line counts the rows by state.
- `reason` joins `state` (`red: no-progress`), which frees one column.
- Turns, context, cost, time and builders are right-aligned.
- The last tool is clipped at 12 characters.
- Each extra stop condition goes on its own `or:` line under `next stop:`.
- The resume lines come last and bare, under `to resume the red or failed rows:`.

No colour and no cursor movement, as before.

## report

Before, the headings were in this order: Merged, Stopped, Burn, Waiting on a hand merge,
Resume, Handovers. The stopped row read `- r1: red ; \`relay resume r1\`` (with a stray space
and semicolon when the reason is empty). The resume lines were list items in backticks. The
burn table was unpadded and had no total.

Checklist:
1. Passed: the title is `# Night <id>: <state>`.
2. Failed: no bare resume line.
3. Failed: Burn came before Waiting and Resume.
4. Failed.
5. The ledger's `stop` line is JSON. §9 asks for it verbatim, so it stays inside a fence as a
   quoted record.
6. That fenced line is over 100 columns. It is exempt as not a human line.
7. Not applicable.

After:

```
# Night 2026-09-22-1: stopped:queue-empty

2 sittings, $2.40: 0 merged, 1 stopped short, 1 waiting on a hand merge.

## Merged

Nothing merged.

## Stopped

The night stopped: queue-empty.

~~~
{"cost_usd":2.4,"event":"stop","night":"2026-09-22-1","reason":"queue-empty","red":1,"resume":["relay resume r1"],"sittings":2,"state":"stopped:queue-empty","ts":"2026-09-22T01:00:00Z"}
~~~

- r1: red

## Waiting on a hand merge

- r2 (off main)

## Resume

~~~
relay resume r1
~~~

## Burn

| row   | state | turns | context |  cost | builders | builder output |
| ----- | ----- | ----: | ------: | ----: | -------: | -------------: |
| r1    | red   |     3 |     71k | $1.20 |        1 |             16 |
| r2    | green |     3 |     71k | $1.20 |        1 |             16 |
| total |       |     6 |         | $2.40 |        2 |             32 |

## Handovers

None.
```

(The report's own fences are shown as `~~~` here.) `TestReportHeadingOrder` pins the order.

## doctor

Before (this machine, with a temp state directory):

```
FAIL claude       ~/.local/bin/claude --help lacks --max-turns
ok   git          /usr/bin/git
ok   git worktree git worktree list runs in $REPO/.claude/worktrees/ux1-review
...
warn queue        $REPO/.claude/worktrees/ux1-review/relay/queue.yaml parses, 6 rows; 6 problems (relay plan names them)
```

The lines above were measured with home written in full. Three lines were over 100 columns:
105, 102 and 147.

Checklist:
1. Failed: there was no verdict line.
5. Failed: home paths were printed whole.
6. Failed.

After:

```
doctor: 1 of 11 checks failed, 2 with a warning
FAIL claude       ~/.local/bin/claude --help lacks --max-turns
...
warn queue        $REPO/.claude/worktrees/ux1-review/relay/queue.yaml parses, 6 rows;
                  6 problems (relay plan names them)
```

The verdict comes first. Home is written `~` at print time, so the `--json` `detail` values are
unchanged. A long detail wraps at spaces under its column (`TestDoctorPrinted`). A single word
over 81 columns, such as a very long path, stays whole. The `--max-turns` failure is AR2-21,
which the owner has not yet decided.

## resume

Before: `relay resume` with no row printed the bare lines, or nothing at all, with no night
line. `relay resume <row>` printed `cd /abs/worktree && claude --resume <id>`.

Checklist:
1. Failed.
5. Failed: the home path was printed whole.

After:

```
night 2026-09-22-1: 1 red or failed row(s) to resume
relay resume r1

cd ~/repo/.claude/worktrees/r1 && claude --resume s-1
```

With no night, `resume` prints `no night yet`. With nothing red, it prints
`night <id>: nothing to resume`. `resume --json` is unchanged.

## attach

Before: `attach --print` printed `tail -f /abs/state/nights/<night>/<row>.log`, or
`tmux attach -t relay-<night>:<row>`. tmux is absent on this Mac (A9).

Checklist:
5. Failed: the state directory was printed with home in full.
The line is a command, so it has no state line to add.

After:

```
tail -f ~/.local/state/agent-relay/nights/2026-09-22-1/r1.log
```

`usage` (`--help`) was two lines: `verbs: plan run attach status ...`. It failed item 7. Now:

```
usage: relay <verb> [--json] [args]
  status  [--night D]                 the night once: state, rows, burn, resume lines
  resume  [<row>]                     reopen a red or failed row's sitting (claude --resume)
  report  [--night D] [--write PATH]  the morning read, in markdown
  attach  [<row>] [--print]           watch the night: its tmux window, or tail -f a row's log
  plan    [--rows a,b] [--until HH:MM]  what a night would run, and whether it is valid
  run     [--rows a,b] [--until HH:MM] [--dry]  the night
          [--max-sittings N] [--max-usd X] [--parallel N] [--words T] [--no-tmux]
  stop                                ask the night to stop after the running sittings
  kill    [<row>]                     stop a row now
  gate    <row>                       the gate by hand
  clean   --merged                    remove the worktrees of merged rows
  doctor                              check claude, git, tmux, the state directory, the config
  version
every verb takes --json
```

## Handover

- Consumer: A8's owner (`internal/night/night_test.go`, `internal/night/plan.go`). Wrap
  plan's and `run --dry`'s `spawn:` line with `\` continuations under 100 columns and write
  home as `~`. Doing this means changing `TestPlanEqualsRun`, which pins the line whole and
  lies outside UX1.
- Consumer: A8's owner (`internal/night/night.go`). The night's per-row line
  `relay r1: red; relay resume r1` should put `relay resume r1` on a line of its own, and
  `relay r1: running in <abs worktree>` should write home as `~`.
- Consumer: A7's or A8's owner (`internal/night/run.go`, `stream.Parse`). The live log writes
  one line per stream event, so an assistant message with two tool uses repeats its line.
  Writing on a change of turn or tool would halve the log.
- Consumer: the owner. The report's `## Stopped` quotes the ledger's `stop` line verbatim as
  JSON (§9) inside a fence, and it runs over 100 columns. The alternative is a key: value
  rendering, which would drop the word "verbatim".
