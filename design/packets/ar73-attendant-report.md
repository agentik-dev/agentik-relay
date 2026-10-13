# Packet AR73: the attendant bounds its tool calls and always answers

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`'s section "The attendant runs out of turns on a check-in" (F9, the observer, 2026-10-11); `plugin/agents/attendant.md` and `internal/packet/ar72_test.go` at f88fd73 |
| pin | agent-relay `main` at f88fd73 plus this packet's own commit; worktree `.claude/worktrees/ar73-attendant-report`, branch `ar73-attendant-report` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night is live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/packet`, `make doclint`, `make hygiene` and `make validate`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `plugin/agents/attendant.md` (its `maxTurns` and one new bullet only), `internal/packet/ar72_test.go` (the `ar72Report` and `ar72Exempt` lines and the comments that name the attendant), `internal/packet/ar73_test.go` (new), `plugin/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json` (their `version` only), `NICE_TO_HAVE.md` (the F9 section's heading only), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/packet -run '^TestAR7[23]_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR7[23]_'` reads 3; `go test ./internal/packet -run '^Test(AR7[23]_\|AR20_)' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^(--- PASS: TestAR7[23]_\|ok )'` reads 4 |
| budget | 40 turns, 80k context, `claude-opus-5-5`; one act, one commit |
| verify | `make check` |

## 1. What the source says (quoted)

NICE_TO_HAVE, F9: "`plugin/agents/attendant.md` keeps `maxTurns: 20` with no report bound (AR72 ruling 4 left it out: \"two verbs, no probe loop\"). The observer's check-in 95 asked it for status, the ledger, tmux, ps, git and the orient charter, and it stopped at 20 turns with no report (24 tool calls) and wrote one on a message. Fix: the same report bound as the adversary's (about 12 tool calls) and maxTurns 30, under TestAR72_1's margin."

AR72 ruling 4, at f88fd73: "The attendant does not, so it is out. Its cap is 20, its job is two `--json` verbs and a few `jq` filters answered in under twenty lines, it has no probing loop, and it has never been seen at its cap." Check-in 95 is that sighting, and it is the new information that reopens ruling 4.

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar73.md` in doclint's five-part shape, opened at the act, closed before the commit, no home path. The act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is.

1. **The attendant joins the report set.** Its callers widen its reading (the observer's brief asks it for the ledger, tmux, ps, git and a charter beside the two verbs), so it has a loop after all, and check-in 95 ended at the cap with no answer. A new bullet goes directly before "Change nothing" (text in §3).
2. **The bound is 15 tool calls and `maxTurns` is 35.** F9's 12 and 30 break AR72's margin (12 + 20 > 30). A check-in's reading is about nine calls (two verbs, `status.json`, a row log, the ledger, tmux, ps, git, a charter), so 15 leaves room for a second look; 15 + 20 = 35 meets the margin. The observer decided this.
3. **The exempt set goes empty, and its rule stays.** `ar72Report` reads `{"adversary", "builder", "attendant"}` and `ar72Exempt` reads `{}`. `ar72ExemptCap` and its loop in TestAR72_2 stay, so a future exempt agent is still held to 20 turns. The comments that call the attendant exempt say it moved under AR73.
4. **One new test.** `TestAR73_1_AttendantReports` in `internal/packet/ar73_test.go` reads a copy of `plugin/agents` through `copyIn`, as TestAR72_1 does. It checks that `ar72Report` holds `attendant`, that `ar72Exempt` is empty, and that the attendant's `maxTurns` is 35 through `ar72MaxTurns`. It is red on the pin, with the message `attendant: in the exempt set, want the report set (AR73 ruling 1)`.
5. **The plugin goes to 0.1.3; no doclint change.** `plugin/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json` both read `0.1.3`. The marketplace is a directory source, so a session loads the plugin from this checkout (the observer's N4, 2026-10-11 20:22). The bump keeps the convention, and the change reaches a session started after the merge. The F9 heading in NICE_TO_HAVE gains `(AR73)`.
6. **Left.** "Answer in under twenty lines", the `model: sonnet` line and every other bullet stay. A caller that wants more reading names its own lower bound.

## 3. Interfaces (signatures, not prose)

No Go signature changes. `attendant.md`'s frontmatter reads `maxTurns: 35`. The new bullet, wrapped at the file's width: "- Count your tool calls. After about 15 tool calls, stop reading and write the answer with what you have; each question you did not finish goes in it, marked unfinished. The answer is never skipped. A caller's bound, if lower, wins." The number is written as digits, directly before "tool calls".

## 4. Findings that name this row

F9: NICE_TO_HAVE's section "The attendant runs out of turns on a check-in" (the observer's check-in 95, 2026-10-11).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory; the installed plugin cache. Read no other repo.

## 6. Report (capped)

At most 120 words. The table: each acceptance line's count, `go vet`'s, `make doclint`'s and `make validate`'s exits, `make hygiene`'s last line. Bullets: the TestAR73 test red on the pin and its line; anything filed. Last line: branch, head sha.
