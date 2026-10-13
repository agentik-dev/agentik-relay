# Packet AR26: packets by a helper, copied in at act 0 (the owner's option A)

| item | value |
|---|---|
| from | the owner's ruling of 2026-09-24 ~18:36 (§1); `design/relay-r1.md` §2, §4, §11, §15, §16; `internal/prompt/`; `internal/night/night.go` 100-125; the decisions tool's `README.md` ("What a record is", "The tool"), in the `claude-code-decisions` checkout beside this repo's |
| pin | agent-relay `main` at the row's start, after `adv-16-relay` merged (cut off `main`, no `base`); worktree `.claude/worktrees/ar26-packet-rule`, branch `ar26-packet-rule` |
| wall | this repo only; other checkouts are read only. No push, `--amend`, rebase, device or network. Every test sets `HOME`, `XDG_CONFIG_HOME` and `XDG_STATE_HOME` to temp dirs; no live `claude` in a test; `go test ./...` only inside `make check`. No `make tla`: the lock and the gate's order do not change. Never `relay plan` or `run` without `--rows` and a temp `XDG_STATE_HOME`. Logs go to `mktemp` |
| may touch | `design/packets/ar26-packet-rule.md` (act 0), `internal/prompt/`, `internal/night/night.go`, the `prompt.Context` literals in `internal/night/run_test.go` and `internal/effects/noprompt_test.go` (if `make check` names them), `design/relay-r1.md`, `decisions/` (new), `.hygiene-allow` (one row), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/prompt -run TestAR26_ -count=1 2>&1 \| grep -c FAIL` reads 0; `grep -rhc '^func TestAR26_' internal/prompt \| paste -sd+ - \| bc` reads 2 or more; `grep -c 'the sitting ends the moment your turn ends' internal/prompt/rules.txt` reads 1; `grep -c 'Packet: r.PacketPath' internal/night/night.go` reads 1; `grep -c 'act 0 copies it verbatim into' design/relay-r1.md` reads 2 or more; `grep -c 'TestAR26_' design/relay-r1.md` reads 2 or more; `test -d decisions; echo $?` reads 0; `grep -c '^# Packet AR26:' design/packets/ar26-packet-rule.md` reads 1; `cat history/sittings/*-ar26.md \| wc -c` under 4096; `grep -cE '/Users/[a-z]\|~/cod[e]/\|filled at close' history/sittings/*-ar26.md` reads 0 |
| budget | 80 turns, 144k context, `claude-opus-5-5` (`--max-turns 80`); one commit per act (0 to 4); `make check` in the foreground, then commit in the same turn; checkpoint at 64 turns: land the act in flight, name the rest in §15, close |
| verify | `make check` |

Stand-in only: `ls decisions/0*.md \| grep -c .` reads 1 or more; `decisions --dir . check` exits 0; `scripts/hygiene 2>&1 \| tail -1` reads `hits outside allowlist: 0`

## 1. What the source says (quoted)

The owner, 2026-09-24 ~18:36, as the orchestrator's program records it: "(packet writers 58 of 183 sittings, $261 of $653): option A: packets are written by a helper agent the orchestrator spawns, in large batches [...], into example-mac relay/prompts; the orchestrator appends the rows; the builder's first act copies its packet into its repo's design/packets; no packet-writer sittings except where a packet needs the target repo's toolchain first; one round after adv-16 makes it the rule (RULES copy-in act in the stand-in and the Go relay, a decision record, relay-r1's packet section)." Run 13's ar25 ended at turn 19: it left act 0 to a background subagent and ended its turn waiting.

## 2. Rulings

You are headless (`claude -p`): the sitting ends the moment your turn ends. Never delegate an act to an agent, never start a background command or a background agent, never end a turn waiting for a notification. Do every act yourself, in the foreground, and commit it before the next act starts.

Each first test is red on `main`. Decide inside the design; the one fork is below.

0. **The copy-in.** Your packet is `relay/prompts/2026-09-24-ar26-packet-rule.md` in the example-mac checkout beside this repo's (the row's `prompt`). Copy it verbatim to `design/packets/ar26-packet-rule.md` (`cp`, `cmp`); commit it alone. The charter opens with act 1: the first free `history/sittings/2026-09-24-<n>-ar26.md` (above ar25c's and adv-16's), titled `# Sitting 2026-09-24-<n>: agent-relay, ar26-packet-rule`, doclint's five rows (goal, budget, pin, window, agents), `## Done-ness`, `## Handover`, `## Next act`.
1. **RULES and CONTEXT.** `rules.txt` gains, right after `You are relay-run, unattended.`, the three headless sentences above, then: "Act 0 comes before any other act: unless the packet the CONTEXT line names already lies inside this repository, copy it verbatim into `design/packets/<row>.md`, `<row>` being your row id, and commit that file alone." Byte-identical to standin-13's; one paragraph, one newline (`TestRender` splits on a blank line). `Context` gains `Packet string`, `contextFormat` gains `; your packet: %s` after the branch clause, `night.go` passes `Packet: r.PacketPath` (inside the repo by §3, so under the Go relay act 0 copies nothing). `TestAR26_RulesCopyInAndHeadless`: both sentences, one paragraph, no `NEXT`, and `TemplateHash()` is not `main`'s `7af0aa7a5c1b`. `TestAR26_ContextNamesPacket`: `Line` carries the path, empty reads `none` (`TestRender`'s count moves to 5).
2. **The design.** §2 gains a paragraph opening **Authored outside, copied in** (AR26, option A): a packet may be authored outside its repo (a helper's batch in example-mac's `relay/prompts/`, the queue row's `prompt` path in the stand-in's program); act 0 copies it verbatim into `design/packets/<row>.md` and commits it alone before any other act, so the repo keeps its packet; one already inside is not copied; the Go queue's `packet` stays inside (§3). §4: act 1's RULES and CONTEXT, the phrase `act 0 copies it verbatim into` again, both template hashes. §11: each `TestAR26_`. §15: the fork. §16: no new model, no `make tla`.
3. **The decision record.** `command -v decisions`; empty: no record by hand, name it in §15. From the worktree root: `decisions --dir . init`; `decisions/governs` gains `packets: where a packet is authored and how it enters its repo` and `prompt: the RULES block and the CONTEXT line`; `decisions --dir . new --title "Packets are written by a helper and copied in at act 0" --governs packets,prompt --consumer design/relay-r1.md --consumer internal/prompt/rules.txt`. Body: `## Words`, §1's ruling quoted; `## Decisions`, each with one `Rejected:` line: (1) a helper writes packets in batches outside the repo, the orchestrator appends rows (Rejected: a packet-writer sitting per row); (2) act 0 copies the packet verbatim into `design/packets/<row>.md`, alone, unless it lies inside (Rejected: the packet left outside the repo); (3) both drivers' RULES carry act 0 and the headless sentences, CONTEXT names the packet (Rejected: the rule per packet); (4) a packet needing the target's toolchain keeps its writer sitting (Rejected: no exception). `## Record`: run 13, 2026-09-24 ~18:36; rows standin-13, ar26. Status stays `proposed`: the status verb is the owner's alone, and the tool's hook denies it to an agent. `decisions --dir . index`. `.hygiene-allow` gains `decisions/*: sibling,private`.
4. **Close.** `make check` green; the charter closed, under 4,096 bytes, no home path, before the last commit; its handover names the owner's step (record 0001 to `in-force` by the tool's status verb) and the fork.

The fork (the owner's): the Go queue refuses a packet outside the repo (`queue.go` 96), so a helper's packet reaches a Go night only once committed there. Recommended, a later row: `packet:` may name a file under one configured packets root, read-only, and the gate checks `design/packets/<id>.md` equals it. Not built here.

## 3. Interfaces (signatures, not prose)

`prompt.Context` gains `Packet string`. `decisions/`: `config`, `governs`, `INDEX.md`, `0001-<slug>.md`.

## 4. Findings that name this row

On `main`: `rules.txt` has neither sentence; `contextFormat` (`template.go` 21) names no packet; no `decisions/`.

## 5. Forbidden reads

Other repos beyond this packet and the README's two sections; the real state root, ledgers, transcripts; worktrees but yours.

## 6. Report (capped)

Each acceptance line's exit and last line; each test red on `main`; the record's path and status; the fork. Last line: branch, head sha.

### 7. Added by the orchestrator (22:05): example-app under the pin, measured

Outside a sitting, `relay doctor --verify <root>/../example-app --json` (temp `XDG_STATE_HOME`, its main bc2cb71): `make check` exit 2 in 6.2 s, last line `lint: it lands in <scratch home>/cache/scratch-*/go/bin (~/go/bin unless GOBIN ...` (staticcheck absent under the pin, as ar25c's charter predicted); `go list -m all | wc -l` exit 0, reads 1, 0.15 s. One more act, after the packet's: relay-r1 §6's paragraph **example-app under the pin** gains these numbers and, per red cause, whose change makes it green (the relay's pin: GOPATH/GOBIN and staticcheck under PinTools; or example-app's Makefile: the owner's); no other change. Acceptance: `grep -c 'exit 2 in 6' design/relay-r1.md` reads 1.

Added 22:12 (standin-13's handover): the two RULES sentences land in `internal/prompt/rules.txt` byte for byte; the source is `RULES` in `<root>/../example-mac/scripts/sitting-relay` (read-only, by `sed -n`), from `You are headless` through `commit that file alone.`
