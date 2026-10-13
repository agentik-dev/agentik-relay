# Packet AR56: status and the report read a hand merge from git; night-open names the revision

| item | value |
|---|---|
| from | the observer's fix queue, "The hand-merge packet and a `revision` key on `night-open`"; night 2026-10-03-1 (§1); `NICE_TO_HAVE.md`, the items on `grem`, g3, "A night's report still lists a row" and "No ledger line records the relay's own revision" |
| pin | agent-relay `main` at the commit that lands this packet; worktree `.claude/worktrees/ar56-hand-merges-read`, branch `ar56-hand-merges-read` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state: a real night may be live. No `make check` and no `go test ./...`: the acceptance lines, `go vet ./internal/night ./cmd/relay`, `make doclint` and `make hygiene` only, one at a time. **Python:** every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/report.go` (`BuildReport`, `HandMerges`), `internal/night/status.go` (`TakeMerges`), `internal/night/night.go` (the `night-open` line, `Revision`; act 3 only), `internal/night/ar56_test.go` (new), `cmd/relay/main.go` (`statusVerb`, `reportVerb`, `resumeVerb`'s list), `cmd/relay/ar56_test.go` (new), `design/relay-r1.md` (§5's `night-open`, the `status` and `report` rows), `plugin/agents/attendant.md` (one sentence), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^TestAR56_" -count=1 -timeout 300s -v 2>&1 \| grep -c "^--- PASS: TestAR56_"` reads 4; `go test ./cmd/relay -run "^TestAR56_" -count=1 -timeout 300s -v 2>&1 \| grep -c "^--- PASS: TestAR56_"` reads 1; `go test ./internal/night -run "^TestAR5[15]_" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./cmd/relay -run "^TestJSONShapesFrozen$" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 50 turns, 150k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

Night 2026-10-03-1, in the consuming repo, ended six rows green and esfix2 red. The owner's session hand-gated all seven green and merged each with plain `git merge`: no `merge` line. Then `relay status` and `relay report` still show esfix2 red with an empty `merge_sha`, six rows "waiting on a hand merge", and `relay resume esfix2` for a row in main. `NICE_TO_HAVE.md` holds it three times; AR51 ruling 6 left it for its own packet.

The cause: `BuildReport` counts a row merged only from a `merge` line (`report.go:107-111`); a green row with none waits (`:138-143`), a red one keeps its resume (`:116-119`). `status` prints `status.json` as the night last wrote it (`status.go:436`).

The shape is git's §3 reading, not a hand gate's line: `relay-r1.md` §3 makes `night.landed` the one merged reading, §5 lets a ledger value only red, and a hand gate never merges. Never bare ancestry: a red row with no commit has its tip on main's line.

The `revision` key (check-in 27) would let a reader tell a night on one build from another; `relay-r1.md` §5 lists the relay's version on `night-open`, and no code writes it.

## 2. Rulings

Not an architecture change: no new ledger event, no new JSON key in `status` or `report`, no new write; the reads go through `effects.Git` and `night.landed`, and no `DECISIONS.md` entry touches them. The charter is `history/sittings/<the build date>-<n>-ar56.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. `DECISIONS.md` stays as it is. Each act's test is red on `main` (one that does not compile there counts); put the red line in the commit message.

1. **The report reads git (act 1).** `HandMerges` maps a row to a merge sha when its last `sitting` line this night is green, red or failed, no `merge` line says `merged`, its `stage` line names a branch that `g.BranchExists` finds in the `night-open` root, and `landed(g, root, or(base, "main"), branch, "")` holds. The sha is the newest merge on main's first-parent line with the branch's tip (`g.Tip`) as a later parent, read by `g.Hermetic(root, "log", "--first-parent", "--merges", "--format=%H %P", "main")`. A git error leaves the row as it is. The `stage` line's base only narrows `landed`, so it only reds. With a git, `BuildReport` lists such a row in `Merged` at that sha: a green one leaves `Waiting`; a red or failed one leaves `Resume`, and its `Stopped.Rows` entry stays, the night's verdict, with no resume line. Without one, as today. `TestAR56_1`, a real root under `t.TempDir()` (`ar13_landed_test.go:16`'s shape, `gitIn`) and literal ledger lines: g1 green and r1 red, each merged `--no-ff` by hand; g2 green, fast-forwarded; r2 red, never moved. With `effects.OSGit{}`, `Merged` is g1 and r1 at their merge shas, `Waiting` is g2 alone, `Resume` is exactly `relay resume r2`.
2. **Status and the verbs (act 2).** `TakeMerges` gives a row in the map with no `merge_sha` that sha and an empty `resume`, and takes its line out of `Status.Resume`. Its `state` and `reason` stay the night's: only the night gated it. `relay status` (not `--watch`) reads the ledger and applies `HandMerges(effects.OSGit{}, lines, n)`; a ledger it cannot read leaves it as today. `relay report` and `relay resume`'s list pass `effects.OSGit{}` to `BuildReport`. `TestAR56_2`: over a literal `Status` (g1 green; r1 and r2 red, each with a resume line), the map {g1, r1} sets both shas, leaves r1 `red`, `Resume` exactly `relay resume r2`, and `PrintStatus` prints no `relay resume r1`. `TestAR56_4` in `cmd/relay` (`fakeEnv`, `main_test.go:922`; a ledger day file and a `status.json` naming a `t.TempDir()` root, r1 merged `--no-ff`): `status --json` and `report --json` give r1 git's sha and no `relay resume r1`.
3. **The relay's revision (act 3).** `night.ReadBuildInfo` is a variable (`debug.ReadBuildInfo`). `Revision()` is the build's `vcs.revision`, `+dirty` appended when `vcs.modified` is `true`, `""` when the build has none (a test binary). The `night-open` line gains `revision`. `TestAR56_3`: with a faked build info (`abc123`, modified), a `nightWorld` night's `night-open` reads `"revision": "abc123+dirty"`; with no `vcs.revision`, `Revision()` is `""`.
4. **The records (act 4).** `design/relay-r1.md`: §5's `night-open` names `revision`; the `status` and `report` rows each gain a sentence: a row landed after its night reads merged at git's sha, no resume line; merge by hand `--no-ff`, as the relay does (D22). `plugin/agents/attendant.md` gains one sentence: a row with a `merge_sha` and no resume line is in main, whatever its state. The four `NICE_TO_HAVE.md` items in the from row each gain "(landed as AR56)".
5. **What this packet does not do.** A hand merge that fast-forwarded main stays waiting (§3: a fast-forward does not land). A row another row replaced (`ovr`) stays waiting. `relay resume <row>` does not refuse a landed row. `status --watch`, the `merge` line, the gates and the hand gate's `night` key do not change.
6. **The review's fix round (act 5).** Review F1: a red row with no commit has its tip at the cut, which a hand merge can make a later parent on main; it loses its resume line. F2: a reused branch name reads the older night's row merged. `HandMerges` skips a row whose tip is its `stage` line's `head_before`, and needs its last `sitting` line's `head_after`, when present, to be an ancestor of the tip or the tip itself. `TestAR56_5` (`t.TempDir()` repos): F1 keeps `relay resume r2`; F2 gives no sha. F3 and F4 go to `NICE_TO_HAVE.md` as the hand-off gives them.

## 3. Interfaces (signatures, not prose)

New: `night.HandMerges(g effects.Git, lines []map[string]any, night string) map[string]string`; `(*night.Status).TakeMerges(m map[string]string)`; `night.ReadBuildInfo` (a variable); `night.Revision() string`; the `night-open` line's `revision`. Changed: `night.BuildReport(lines []map[string]any, night string, g ...effects.Git) Report`. Unchanged: `Report`'s and `Status`'s fields and JSON keys, `landed`, `Markdown`, `PrintStatus`.

## 4. Findings that name this row

`internal/night/report.go:85-143`; `internal/night/status.go:24-62`, `:436`; `internal/night/plan.go:683` (`landed`); `internal/night/night.go:543-546` (`night-open`), `:920-925` (the precedent: `landed` sets a merge sha); `internal/effects/effects.go:20-27`, `:53`; `cmd/relay/main.go:1358`, `:1612`, `:1776`; `design/relay-r1.md:74`, `:97`, `:208`, `:212`.

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line with its count, `go vet`'s exit, `make doclint`'s exit and `make hygiene`'s last line. Bullets: each act's red line on `main`; what was left. Last line: branch, head sha.
