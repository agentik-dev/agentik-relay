---
status: current
updated: 2026-10-12
---
# agentik-relay: what is settled

The closed choices of this repository: one numbered entry each, one paragraph, in plain English, with the date and the source it condenses. An entry here is closed unless new information breaks it. A review may block only on unmet acceptance criteria, bugs or failing tests, or a conflict with an entry here; everything else goes to NICE_TO_HAVE.md and does not block. (The owner's word, 2026-09-25.)

## In force

### Packets

1. **A row may declare its packet copy-in skipped, and the gate then compares nothing.** When a packets source is configured, the gate compares the packet committed under `design/packets/` against the source for every row, and a mismatch or a missing copy reads red. A row that leaves its packet at its source on purpose, as a review row does when its prompt names a path the public-hygiene scan refuses in this repository (the adv-16 precedent, dd8bd3f), declares the copy-in skipped in its queue row; for that row the gate runs no comparison and records the check as skipped. The field's name and its place in the queue schema are the builder's. (2026-09-25; the ar29-fixes-adv17 packet's §15 question, settled as a reversible choice by the portfolio's management sitting 2026-09-25-4 under the owner's fold-and-rewrite prompt, Part A, item 3)

2. **Packets are written before their rows and copied in at act 0.** A packet is written before its row runs, by the sitting that cuts it or by a helper agent that sitting spawns; no row gets a packet-writer sitting, since those sittings took 58 of run 13's 183 sittings and $261 of its $653. The Go queue's `packet` lies inside the repository (relay-r1 §3), so the cutting sitting commits the packet at `design/packets/<row>.md` on main, with its queue row, before the night. Act 0 comes before any other act: a packet that lies outside the repository is copied verbatim into `design/packets/<row>.md` and committed alone, so the history shows what each row was asked. `internal/prompt/rules.txt` carries act 0 and the headless sentences, and the CONTEXT line names the packet. A packet that needs the target repository's toolchain before it can be written keeps its packet-writer sitting. Every packet follows relay-r1 §2, its writer's appended notes included: under 9,216 bytes, six sections, and acceptance on the allowlist. (2026-09-24, run 13, the owner's option A on the orchestrator's cost read; rows standin-13 and ar26-packet-rule, whose act 0 found the verbatim copy did not parse; folded 2026-09-28 from record 0001, which left with the `decisions/` directory; amended 2026-09-28, the example-mac stand-in's retirement, example-mac DECISIONS.md entry 5 as amended by the owner's answer 5 of 2026-09-28: the helper's batches into example-mac `relay/prompts/` and the byte-for-byte parity with the stand-in's RULES lapse, charter example-mac 2026-09-28-6)

### The orchestrator row

3. **Two repos at once.** One orchestrated night serves one repo. The account's lock allows one relay at a time, so while one repo's orchestrator runs as a night, any other repo's orchestrator stays attended. (2026-09-29; the owner's answer to decision 3, given in the selector, example-mac sitting 2026-09-28-6; design/relay-orchestrator-r1.md)

4. **Who pushes after a night.** After an orchestrated night's merges, the relay runs the repo's full check in a linked worktree of the root and, when the row says `push: main`, pushes main through the repo's pre-push gate. A relay child still never pushes. (2026-09-29; the owner's answer to decision 4, given in the selector, example-mac sitting 2026-09-28-6; design/relay-orchestrator-r1.md)

5. **The owner's answers during a night.** While a night runs, the owner answers with `agentik relay words "<text>"`, which only the owner's terminal may run: a row and a helper are refused. The next orchestrator sitting receives the text and records it with `agentik decisions answer`. Stopping the night, answering in the file, committing and starting again always works too. (2026-09-29; the owner's answer to decision 5, given in the selector, example-mac sitting 2026-09-28-6; design/relay-orchestrator-r1.md)

6. **Writes to other repos.** An orchestrator row writes only its own repo. It reads a sibling repo as plain files and files a cross-repo ask as an intake file in its own `history/sessions/`. Work that writes several repos, such as example-ops's cross-repo moves, stays in an attended sitting. (2026-09-29; the owner's answer to decision 6, given in the selector, example-mac sitting 2026-09-28-6; design/relay-orchestrator-r1.md)

7. **Notices keep quiet hours.** Notices keep quiet hours, not nights: the config names a quiet window (the owner's is 21:00 to 07:00, America/Los_Angeles), and a notice raised inside it goes to the log only. When the window ends and the run is still going, one catch-up notice gives the count of those held, never their text; outside the window, an unattended run notifies as it goes, whatever the hour it started. An orchestrated run refuses to start with no ntfy topic unless it is started with `--no-notify`, which keeps every notice of that run in its log. (2026-09-29; the owner's answer to decision 7, the owner's word in the selector, example-mac sitting 2026-09-28-6)

### The guards

8. **The guard's shell reader.** Inside a relay row, the guard parses each Bash command with the mvdan.cc/sh Go library and judges every command in the syntax tree. A line the parser rejects is denied inside a row, and so is a zsh-only spelling the parser does not know. agentik-relay takes mvdan.cc/sh as its first outside dependency. The hand-written reader in internal/hooks/guards.go is replaced by packets that follow a design record with one refuter round; until they land, the reader stays as AR44 left it and takes no further patch. (2026-09-30; the owner's answer to decision 8, the owner's word in the observer session, 2026-09-30 21:00)

9. **The guard's zsh test table.** The owner writes the table of about twenty zsh-only spellings, each shown read or denied, in one test file and at a time of the owner's choosing; no helper writes it, because Claude Code's safety check stops a helper that composes those lines. The tree reader (AR46, merged at 98f488c) stays live meanwhile, and the table stays an open risk in design/guard-parser-r1.md. Packet 3, which deletes the hand-written reader, waits for the table. (2026-10-10; the owner's answer to decision 9, the owner's word in the observer session, 2026-10-10 15:37)

### The gate

10. **The gate's verify time limit.** A repo's relay config may carry its own verify limit; when it is unset, the limit stays 900 s. `agentik relay doctor --verify` measures the repo's verify line and suggests a limit. A verify line cut at its limit reads red with a reason that says the line was cut, never with an empty reason. (2026-10-10; the owner's answer to decision 10, the owner's word in the observer session, 2026-10-10 15:37)

### Usage limits

12. **When one model's limit is hit, should only that model's rows pause.** When one model's own limit is hit and names its reset, only that model's rows pause; rows on other models keep running, and the paused rows continue at the reset (packet AR71). The five-hour window's limit still pauses the whole night, as the owner ruled on 10-10. Portfolio entry 23's amendment gains this sentence. (2026-10-12; the owner's answer to decision 12, the owner's word in the observer session, 2026-10-12, entered 03:18)

11. **Should a night switch to a second model when one model's limit is hit.** A repo's relay config may name `limit_fallback_model`; when it is unset, nothing changes. When it is set, a row stopped by its own model's limit continues at once on that model, in its own tree (packet 7). Only a less critical repo sets it; example-app never does. (2026-10-12; the owner's answer to decision 11, the owner's word in the observer session, 2026-10-12, entered 03:18)

## Proposed
