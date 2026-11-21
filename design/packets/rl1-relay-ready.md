# Packet RL1: the relay's gate reads no record of its clone

| item | value |
|---|---|
| from | the coordinating repo's row brief `rl1-relay-ready`, the owner's words below; its measure: six Go tests walk the real `design/packets` and fail once the records leave the clone |
| pin | `main` at `a2e6ec7` plus this packet's own commit; branch `rl1-relay-ready`, worktree `.claude/worktrees/rl1-relay-ready`; nothing linked from main (a Go repo, no venv) |
| wall | this repo's worktree only; no push, `--amend`, rebase, merge, install, plugin update; never build a relay binary by hand, never run one outside `make`'s test harness, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; no `claude`; `make check` only through the gate queue, the rehearsal's in a scratch clone outside every checkout; no `DECISIONS.md` or rendered block edited |
| may touch | `internal/packet/ar10_test.go`, `internal/packet/ar17c_test.go`, `internal/packet/ar20_test.go`, `internal/packet/ar21_4_test.go`, `internal/packet/ar61_test.go`, `internal/queue/ar17d_test.go`, `internal/packet/testdata/packets/` (new), `internal/packet/testdata/peract/` (new), `internal/queue/testdata/rows/` (new); any other test or script R3's rehearsal finds reading a record of the clone, each named in §4; `design/packets/rl1-relay-ready.md`, `history/sittings/2026-11-21-1-rl1-relay-ready.md` |
| acceptance | `go test ./internal/packet -run '^(TestAR10_0_ParseRefusesUnboundedLines\|TestAR17C_0_LivePacketsParse\|TestAR20_5_SpentStems\|TestAR21_4_OwnLinesPass\|TestAR61_6_CorpusQuiet)$' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads `5` (main: `5`); `go test ./internal/queue -run '^TestAR17D_3_RowsFilesRepoRelative$' -count=1 -v 2>&1 \| grep -c '^--- PASS'` reads `1` (main: `1`); `grep -rhF 'copyIn(t, "../..", "design' internal cmd \| wc -l` reads `0` (main: `6`) |
| budget | 90 turns, 200k context, `claude-opus-5-5`; four acts, one commit each: 0 this packet; 1 R1; 2 R2; 3 R3's rehearsal and what it turns red |
| verify | `make check` |

## 1. What the source says

The owner's entry 63 of the coordinating repo's ledger, verbatim: "Each of agentik's public tools
keeps its working records in agentik-hq. As a tool's row lands, its charters, packets, wish list
and questions move to agentik-hq/repos/<tool>/ and leave the public tree in one commit; what is
already pushed stays in the public history (design/28)." Entry 37, verbatim: "Never commit. The
tool repos are public. Commit no hostname, address, email, person's name, account detail, or
private repo's name or path; examples use made-up names." This row moves no record: it makes the
gate green without them, so the move can come.

## 2. Rulings

1. **R1, one packet named.** `ar10_test.go` and `ar21_4_test.go` each read one packet by name, a
   copy under `internal/packet/testdata/packets/` with the bytes it has at the pin, and assert what
   they assert today.
2. **R2, every packet globbed.** `ar17c_test.go`, `ar20_test.go` and `ar61_test.go` read every
   packet, `internal/queue/ar17d_test.go` every `rows-*.json`: each reads a fixed sample under
   `testdata`, the fewest files with which each assertion still fails when the code it guards is
   broken; a pinned count becomes the sample's. Never the whole set. A fixture the hygiene scan
   refuses holds made-up names in the refused words; no allow row is added here.
3. **R3, the rehearsal.** A scratch clone of this branch, outside every checkout, with every
   tracked record (`history/`, `design/packets/`, `bugs/`, `HANDOFF.md`, `NICE_TO_HAVE.md`,
   `OPEN-QUESTIONS.md`, `relay/queue.yaml`) removed and `records: out` in `.hygiene-allow`,
   committed there alone, runs the whole `make check` through the gate queue. A red that is a test
   or script here reading a record of the clone is mended as R1 and R2, one commit each, a line in
   §4; any other red stops the row.
4. **R4, what stands.** No assertion weakens, none is skipped; nothing outside test files and
   `testdata` changes, or each such change is named in §4 with its reason; in a tree that holds
   its records every verb and test reads as at the pin.

## 3. Interfaces

No code interface changes. The samples: `internal/packet/testdata/packets/` (R1's two and the
spent sample), `internal/packet/testdata/peract/` (`TestAR61_6_CorpusQuiet`'s), and
`internal/queue/testdata/rows/` (`TestAR17D_3_RowsFilesRepoRelative`'s), each file a copy of the
record at the pin under its own name, since `Parse` takes the id from the file stem.

## 4. Findings that name this row

The rehearsal's reds beyond the six, each with its commit, are listed here as they are mended.

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or run record;
transcripts; the coordinating repo's row file.

## 6. Report

The branch and head sha; the gate's ids and their lines, the rehearsal's among them; each
acceptance line at the pin and now; each sample's files and the assertion each keeps alive.
