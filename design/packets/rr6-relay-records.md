# Packet RR6: the relay refuses a repo whose records live outside its clone

| item | value |
|---|---|
| from | agentik-hq's row brief `rr6-relay-records`, the owner's words below; the coordinating ledger's entry 65 |
| pin | `main` at `b735980` (plugin 0.2.4) plus this packet's own commit; branch `rr6-relay-records`, worktree `.claude/worktrees/rr6-relay-records`; nothing linked from main (a Go repo, no venv) |
| wall | this repo's worktree only; no push, `--amend`, rebase, merge, install, plugin update; never build a relay binary by hand, never run one outside `make`'s test harness (so `go test ./cmd/agentik-relay` runs only inside the gate's check), never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; no `claude`; `make check` only through the gate queue; no `DECISIONS.md` or rendered block edited; no resolver ported, no table of cases vendored |
| may touch | `internal/effects/records.go` (new), `internal/effects/records_test.go` (new), `cmd/agentik-relay/main.go`, `cmd/agentik-relay/rr6_test.go` (new), `cmd/agentik-relay/main_test.go` (`TestDoctorTable`'s names and letters, R3), `internal/packet/agentik_r1_test.go` (the plugin's version pin, R5), `plugin/.claude-plugin/plugin.json`, `README.md`, `design/readme/source.md`, `design/readme/rendered.sha256`, `design/packets/rr6-relay-records.md`, `history/sittings/2026-11-20-1-rr6-relay-records.md` |
| acceptance | `go test ./internal/effects -run '^TestRR6_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestRR6_'` reads `3` (main: `0`); `go test ./cmd/agentik-relay -run '^TestRR6_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestRR6_'` reads `6` (main: `0`), run inside the verify row's check only; `grep -c -F 'agentik.records' design/readme/source.md` reads `1` (main: `0`); `grep -c -F 'agentik.records' README.md` reads `1` (main: `0`); `grep -c -F '"version": "0.2.5"' plugin/.claude-plugin/plugin.json` reads `1` (main: `0`) |
| budget | 90 turns, 200k context, `claude-opus-5-5`; four acts, one commit each: 0 this packet; 1 the tests, red; 2 R1 to R4, green; 3 R5 |
| verify | `make check` |

## 1. What the source says

The owner, 2026-11-20, asked whether the relay should learn to work with records outside the
clone now or refuse such a repo for now: "Refuse for now".

The coordinating ledger's entry 65: "The relay does not plan, run or gate in a repo whose records
live outside its clone: it says so in one line that names the key, and its doctor shows the
state." The relay reading such a repo's records through a resolver is not built here. This
repo's `DECISIONS.md` binds; no entry of it changes.

## 2. Rulings

1. **R1, the one question.** A repo's records are out when, at the repo's root, `git config
   --local --get agentik.records` prints a value (exit 0, whatever the value), or `.hygiene-allow` at `HEAD`, read through git, holds the whole line `records: out`
   (trailing white space stripped, as commons' `_declares` reads it). Else they are the repo's
   own; a git failure reading either is the repo's own. One function, through the hermetic runner.
   The coordinator's ruling (2026-11-20, build 2): the key's presence decides, asked as `git
   config --local --get` with no `--path`, so a value `--path` cannot expand (`~nouser/x`, exit
   128), which commons reads `broken`, is refused and shown by the doctor like any set key; a git
   failure not about the key (no repo, a config git cannot parse) stays the repo's own. The repo's
   root is the main checkout: from a linked worktree the relay reads the main checkout's config and
   `HEAD`, on purpose, since it plans and merges there; commons reads the worktree's own top.
2. **R2, the refusal.** `plan`, `run` and `resume` ask R1 after their argument checks and before
   the pid file, the lock, the ledger or any stage; `gate` asks it just after it takes the lock,
   since ADV13-14 (`TestAR22_2_HandGateHoldsLock`) has the hand gate read no root while another
   relay holds it, and its refusal releases the lock and removes the pid file it wrote. Out: one
   line on stderr, exit 2 (a failed pre-check): `relay: this repo keeps its records outside its
   clone (git config agentik.records); the relay does not run in such a repo yet`. Nothing of the
   verb's own is made or written on a refusal: the start-up pin, which R1's git needs, still makes
   `<state>/home/` before the verb, and the gate asks after it takes the lock, as ADV13-14 forces.
3. **R3, the doctor.** One row, `records`, after `root`: `ok`, `in the repo`; or `warn`,
   `outside the clone: the relay does not run here (git config agentik.records)`. Text and JSON.
4. **R4, what stands.** In a repo whose records are its own, every verb reads as at the pin; no
   other verb, hook or guard asks R1.
5. **R5, the words.** The README's one sentence beside where the queue lives, in both files, and
   the sidecar; the plugin at 0.2.5 with its version pin, the last commit under `plugin/`.

## 3. Interfaces

`effects.RecordsOut(root string) bool` (`internal/effects/records.go`): R1. `effects.Declares(text
string) bool`: a file's text holds the line. `main.recordsRefused(stderr io.Writer) bool`: the root
(`toplevel`, then `OSGit.Root`) asked R1, the line printed. The runner's env is the pin's own, so
neither `GIT_CONFIG_*` nor a global file reaches it; `--local` reads the clone's common config,
so a worktree sees the main checkout's key, and `HEAD` is the main checkout's.

## 4. Findings that name this row

None yet. Not built: a resolver for out records (design/28 item 17), the relay's own records'
move, a broken key's reasons.

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or run record;
transcripts; the coordinating repo's row file.

## 6. Report

The branch and head sha; the gate's id and its line; each acceptance line at the pin and now; how
`_declares`' edge lines read; choices; leftovers.
