# Packet AR68: the continuation's prompt and start hook, and the orchestrator sitting at the wake

| item | value |
|---|---|
| from | `design/usage-limit-resume-r1.md` §5 packet 4, its second half (§2's RESUME block, start hook, orchestrator sitting at the wake, `nextSitting`'s red list and `orc.stuck`); `NICE_TO_HAVE.md`'s verify-round note; `design/relay-r1.md` §1, §4, §7, §10 |
| pin | agent-relay `main` after AR67's merge (it moves `night.go`; this row depends on AR67); worktree `.claude/worktrees/ar68-limit-wake-orchestrator`, branch `ar68-limit-wake-orchestrator` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet ./internal/night ./internal/hooks ./internal/prompt`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/prompt/resume.go` (new), `internal/hooks/start.go`, `internal/night/run.go` (the child's environment only), `internal/night/night.go`, `internal/night/wake.go`, `internal/prompt/ar68_test.go` (new), `internal/hooks/ar68_test.go` (new), `internal/night/ar68_test.go` (new), `internal/night/ar67_test.go` (an orchestrator case only, if AR67 wrote one), `design/relay-r1.md` (§1, §4, §7, §10, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^Test(AR68_\|AR67_\|AR65_\|AR66_\|AR32A_4_)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR68_\|ok )"` reads 3; `go test ./internal/prompt -run "^Test(AR68_\|Render\|TemplateHash)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR68_\|ok )"` reads 2; `go test ./internal/hooks -run "^Test(AR68_\|AR23_\|Run)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR68_\|ok )"` reads 2; `go test ./cmd/relay -run "^TestAR59_4_Gofmt" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The record, §2: "The prompt: the row's own and a RESUME block naming the part, the last session, the commits since `head_before` (at most 20) and the uncommitted paths (at most 40): these files are yours; read them, keep what is sound, continue from the first act your commits do not cover." "The start hook records `head_before`, not the new HEAD (`internal/hooks/start.go:33-38`), so a part with nothing left is not nudged for `no-commit` (`stop.go:47-53`)." "An orchestrator sitting is not continued; its next sitting is cut as orchestrator r1 §4 cuts any." "`redSittings` [...], `nextSitting`'s red list [...] and `orc.stuck` skip that line, so a recovered night parks nothing". The verify round: "No packet owns `nextSitting` or `orc.stuck`; name an owner." AR66 answered there: "packet 4 owns both, since only its wake cuts a sitting after a pause and a paused sitting never merges, so `judge` never counts it".

The code at `4814043` (AR67 moves `night.go`): `Render` ends at the packet (`internal/prompt/template.go:103`); a fresh start records `fx.Head` (`internal/hooks/start.go:38-39`); `setRow` gives a failed row its resume (`internal/night/night.go:707`); `nextSitting`'s red list takes every red or failed sitting (`:828`), and so do the stop line's `red` and `resume` (`:1402`) and `Clean` (`:1448`); `judge` runs only after a merge (`:1005`).

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar68.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message.

1. **Act 1, the RESUME block** (`prompt.ResumeBlock`, pure): `RESUME: part <n>; the last session <id> stopped at the account's usage limit; this part's ceilings: <t> turns, $<u>.`, the commits since the cut (newest first, at most 20, then `(+<k> more)`), the uncommitted paths (at most 40, likewise), `none` when empty, and the record's sentence. Outside the template hash. A continuation's stdin is the row's, a blank line, then the block (commits by `Git.Hermetic` `log` from `head_before`, paths by `Git.Status`, the ceilings `remainder`'s), so the child reads the part's ceilings, not only CONTEXT's.
2. **Act 1, the start hook.** `hooks.HeadBeforeVar` is `RELAY_HEAD_BEFORE`. In a sitting, a start records the variable as `Head` when it is set, else `fx.Head` as today; a `resume` or `compact` with an old head keeps it as today, one without records as a startup does. So a continued part with nothing left to commit is not nudged `no-commit`.
3. **Act 1, the child's environment.** `RunRow` adds `RELAY_HEAD_BEFORE=<Staged.HeadBefore>` to every part's environment after `ChildEnv` (which drops a parent's `RELAY_*`), so a first part records the head it records today.
4. **Act 2, the orchestrator sitting at the wake.** It replaces AR67's ruling 4 stop: after `--until` and `--max-usd`, before the other checks, a paused orchestrator sitting is halted (`Step(Paused, Halt)`, failed `usage-limit`, its stage unlocked, its tree kept), never continued, and printed with no `relay resume` line. The loop then cuts the standing row's next sitting as orchestrator r1 §4 cuts any (`count` and `--max-sittings` as today).
5. **Act 2, a recovered sitting.** `Night.Woken` holds each sitting ruling 4 halts. `nextSitting`'s red list, the stop line's `red` and `resume`, `setRow`'s resume and `Clean` skip it, so the next sitting and the next night read it recovered. `orc.red` and `orc.stuck` stay as they are: a halted sitting sends no outcome and never merges, so `judge` never counts it.
6. **Tests**, each naming the variable as the literal `RELAY_HEAD_BEFORE`. prompt: `TestAR68_1_ResumeBlock` (25 commits, 45 paths: 20 and `(+5 more)`, 40 and `(+5 more)`; empty, `none`; the ceilings named; `TemplateHash` unchanged). hooks: `TestAR68_2_StartHookCut` (in a row, HEAD at sha1, a `startup` start with the variable at sha0 records sha0, and Stop on the clean tree does not block; without it, sha1 and Stop blocks `no-commit`; a `resume` keeps the old head). night: `TestAR68_3_ContinuationPrompt` (`TestAR67_3`'s world, r1's `a.txt` edited again after the test's commit: r1's part 2 stdin ends in a RESUME block naming part 2, part 1's session, the commit's subject, `a.txt`, 60 turns and $5; every part's `Env` holds `RELAY_HEAD_BEFORE=` its `stage` line's `head_before`, once). `TestAR68_4_OrchestratorWake` (`TestAR32A_4_RestartsAfterMerge`'s world on `effects.StepClock`, its spawner handing o.1 `TestAR65_3`'s stream (a `FakeSpawner`, exit 143) and the rest to `effects.OSSpawner`, o.2's charter `DONE:`: paused, then the clock to the wake: one `wake` line; o.1 failed `usage-limit`, unlocked, its tree there, one spawn; o.2 spawned, a new session, its `SittingStart.Red` (read in `Row`) without o.1; `stopped:done`; the stop line's `red` 0 and `resume` empty; `night.Clean` true; no `relay resume o.1` in the night's output).
7. **The record, in act 3.** relay-r1 §1: the orchestrator sitting halted at the wake and its next cut; §4: the RESUME block; §7: the child's `RELAY_HEAD_BEFORE`; §10: SessionStart's head on a continuation; §11: the tests. The verify-round note gains `(AR68)`.
8. **Not done.** The guards (packet 5): the pause count, the same reset, a model's scope, a sibling gated again. The readers (packet 6): status, report (`report.go:151` still prints `relay resume` for a woken sitting) and the stop verb's words. `Night.tla`.

## 3. Interfaces (signatures, not prose)

`prompt`: `type Resume struct { Part, Turns, USD int; Session string; Commits, Paths []string }`, `func ResumeBlock(r Resume) string`. `hooks`: `const HeadBeforeVar = "RELAY_HEAD_BEFORE"`. `night`: `Night.Woken map[string]bool`. Red on the pin: `_1` (undefined), `_2` (the hook records HEAD), `_3` (no RESUME block, no variable), `_4` (the night stops at the wake: `o.1: an orchestrator sitting is not continued`).

## 4. Findings that name this row

The verify round: no packet owned `nextSitting` or `orc.stuck`; AR66 named packet 4. The record's §2: an orchestrator sitting is not continued at the wake. AR67's refuter (N3): CONTEXT names the row's turns, not the part's.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line with its count, `go vet`'s exit, `make doclint`'s exit and `make hygiene`'s last line. Bullets: the TestAR68 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
