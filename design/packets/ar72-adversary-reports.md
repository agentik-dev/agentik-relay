# Packet AR72: an agent that owes a report bounds its tool calls

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`'s section "The adversary agent runs out of turns before its report" (F4, the observer, 2026-10-11); `plugin/agents/adversary.md`, `plugin/agents/builder.md`, `plugin/agents/attendant.md` at eb13373 |
| pin | agent-relay `main` at eb13373 plus this packet's own commit; worktree `.claude/worktrees/ar72-adversary-reports`, branch `ar72-adversary-reports` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/packet`, `make doclint`, `make hygiene` and `make validate`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `plugin/agents/adversary.md` (its `maxTurns` and one new bullet only), `plugin/agents/builder.md` (its two budget bullets only), `internal/packet/ar72_test.go` (new), `plugin/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json` (their `version` only), `NICE_TO_HAVE.md` (the F4 section's heading only), `history/sittings/*.md`. Nothing else: `plugin/agents/attendant.md` and `scripts/doclint` stay |
| acceptance | `go test ./internal/packet -run '^TestAR72_' -count=1 -timeout 300s -v 2>&1 \| grep -c '^--- PASS: TestAR72_'` reads 2; `go test ./internal/packet -run '^Test(AR72_\|AR20_)' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^(--- PASS: TestAR72_\|ok )'` reads 3 |
| budget | 40 turns, 80k context, `claude-opus-5-5`; one act, one commit |
| verify | `make check` |

## 1. What the source says (quoted)

NICE_TO_HAVE, F4: "`plugin/agents/adversary.md` sets `maxTurns: 80` and asks for no report checkpoint, so a long refutation ends at the cap with no report and needs a message to write one." "A prompt line saying 'report by turn 70' did not prevent it, because the agent cannot see its turn count. Fix: a rule in the agent body that bounds the probes by count (for example, after about 50 tool calls, write the report with what you have), and perhaps `maxTurns: 100`." The observer, 2026-10-11, in this packet's brief: the cap was hit four times that day, and a caller's line "report within 50 tool calls, even if partial" worked four times in four.

The agents at eb13373. `adversary.md`: `maxTurns: 80`; its body ends "End with the count of findings and the slice's head sha", with no bound. `builder.md`: `maxTurns: 120`; "Past 80% of the packet's budget, land the act in flight at a checkpoint and stop" (a packet's budget is in turns, e.g. "70 turns"); "Report: one table with each acceptance line, its exit and its last output line". `attendant.md`: `model: sonnet`, `maxTurns: 20`; "Run `relay status --json` and `relay report --json`"; "Answer in under twenty lines".

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar72.md` in doclint's five-part shape, opened at the act, closed before the commit, no home path. The act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is.

1. **The adversary's bound.** A new bullet before "End with the count of findings" (text in §3): after about 50 tool calls it stops probing and writes the report with what it has; each unfinished check goes in the observations list marked unfinished; the report is never skipped; a caller's bound, if lower, wins. The bound is counted in tool calls because the agent can count its own calls and cannot see its turns; this is the shape that worked four times in four.
2. **`maxTurns: 100`.** Every turn that uses a tool carries at least one call, so 50 calls spend at most 50 turns and leave 50 for the report. The test asks a margin of 20 (N + 20 <= maxTurns).
3. **The builder carries the same risk, so it is in.** Its report is required, its cap is 120, and its only stop, "Past 80% of the packet's budget", is counted in turns it cannot see: the "report by turn 70" line that failed. That bullet is kept, and a bound of about 90 tool calls joins it (text in §3), whichever comes first; `maxTurns` stays 120, which leaves 30 turns to land the act, commit and report. A packet's or a caller's lower bound, given in tool calls, wins.
4. **The attendant does not, so it is out.** Its cap is 20, its job is two `--json` verbs and a few `jq` filters answered in under twenty lines, it has no probing loop, and it has never been seen at its cap. A margin of 20 cannot fit under a cap of 20. The test names it exempt and holds its cap at 20 or less, so raising the cap needs a ruling.
5. **Where the test goes.** `internal/packet/ar72_test.go`. No package tests `plugin/` today. `internal/packet` already holds the repo-file checks (`ar52_test.go` reads the Makefile, `ar20_test.go` and `ar17c_test.go` read `design/packets`) through `copyIn`. Doclint's plugin rule checks frontmatter fields only, and its bytes are pinned (conventions §6), so it cannot grow here. `make check` runs the test through `go test -race ./...`. The frontmatter is parsed with `internal/yaml`'s `Parse`, which reads all three agents' frontmatter at eb13373.
6. **Tests.** `TestAR72_1_ReportBound`. For `adversary` and `builder`: the frontmatter between the two `---` lines is parsed, and `maxTurns` is read as an int (adversary 100, builder 120). The body is joined on whitespace. It holds `\b(\d+) tool calls\b`, the first N has N + 20 <= maxTurns, and the same bullet holds "never skipped". Red on the pin: `adversary.md: no "<N> tool calls" bound in the body`. `TestAR72_2_AgentsClassified`: the names of `plugin/agents/*.md` are exactly the report set {adversary, builder} plus the exempt set {attendant}, and each exempt agent's `maxTurns` is at most 20. It passes on the pin, and it fails when a new agent is added without a ruling.
7. **The plugin goes to 0.1.2; no doclint change.** `make install` runs only `go install ./cmd/relay`, and `claude plugin update` caches by version: the installed 0.1.1 is the copy of 09-30, and AR60 to AR62's builder, attendant, plan and status changes never reached a session. `plugin/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json` both read `0.1.2`, which carries those changes and this row's. The F4 heading in NICE_TO_HAVE gains `(AR72)`. (The observer's amendment, 2026-10-11.)
8. **Left.** A hook that counts a subagent's tool calls and warns at its bound would not depend on the agent counting. It is out of scope and goes to NICE_TO_HAVE only if this rule is seen to fail.

## 3. Interfaces (signatures, not prose)

No Go signature changes. `adversary.md`, the new bullet, wrapped at the file's width: "- Count your tool calls. After about 50 tool calls, stop probing and write the report with what you have; each check you did not finish goes in the observations list, marked unfinished. The report is never skipped. A caller's bound, if lower, wins." `builder.md` keeps "- Past 80% of the packet's budget (turns or context), land the act in flight at a checkpoint and stop." and adds after it: "- Count your tool calls. After about 90 tool calls, land the act in flight at a checkpoint, commit it, and write the report with what is done; the acts left go in the charter's handover. The report is never skipped. Whichever of this bound and the 80% checkpoint comes first holds; a packet's or a caller's lower bound, given in tool calls, wins." (fix round one, F1: the 80% checkpoint is kept) The number is written as digits, directly before "tool calls".

## 4. Findings that name this row

F4: NICE_TO_HAVE's section "The adversary agent runs out of turns before its report" (AR68's packet refuter, AR68's round-one design lens, the AR70 and AR71 refuter, and the fourth case in the observer's brief).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory; the installed plugin cache. Read no other repo.

## 6. Report (capped)

At most 120 words. The table: each acceptance line's count, `go vet`'s, `make doclint`'s and `make validate`'s exits, `make hygiene`'s last line. Bullets: the TestAR72 test red on the pin and its line; anything filed. Last line: branch, head sha.
