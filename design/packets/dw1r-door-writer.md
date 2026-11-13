# Packet DW1R: `make install` leaves a newer `agentik` door in place

| item | value |
|---|---|
| from | agentik-hq's row brief `dw1r-door-writer` (a mend, found by a reproduction; no owner's words); the suite's canonical install module, which writes the door forward only |
| pin | `main` at `7359e5b` plus this packet's own commit; branch `dw1r-door-writer`, worktree `.claude/worktrees/dw1r-door-writer`; nothing linked from main (a Go repo, no venv) |
| wall | this repo's worktree only; no push, `--amend`, rebase, merge, real `make install`, plugin update; never build a relay binary by hand, never run one outside `make`'s test harness, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; never `claude`; `make check` only through the gate queue (`agentik session gate run`), never a bare `make`; no `DECISIONS.md` or rendered block edited; `scripts/agentik` and every vendored file unchanged |
| may touch | `scripts/install-door` (new), `Makefile` (the `install` target's door line and its `##` text), `cmd/agentik-relay/dw1_test.go` (new), `cmd/agentik-relay/agentik_r1_test.go` (`TestAgentikR1_5_InstallTarget`'s fourth line), `design/packets/dw1r-door-writer.md`, `history/sittings/2026-11-13-1-dw1r.md`; at review 1, `NICE_TO_HAVE.md` (its open rows) |
| acceptance | `go test ./cmd/agentik-relay -run '^TestDW1_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestDW1_'` reads `6` (main: `0`); `grep -c -F 'sh scripts/install-door scripts/agentik' Makefile` reads `1` (main: `0`); `go test ./cmd/agentik-relay -run '^TestAgentikR1_5_' -count=1` exits 0 (main: 0, on the old line); `go test ./internal/hygiene -run '^TestAgentikR1_2_' -count=1` exits 0 (main: 0, its six lines OK); `git diff main -- scripts/vendored.sha256 scripts/agentik plugin \| wc -l` reads `0` (main: `0`) |
| budget | 40 turns, 150k context, `claude-opus-5-5` at `high`; three acts, one commit each: 0 this packet; 1 the tests, red; 2 the script and the Makefile's line, green |
| verify | `make check` |

## 1. What the source says

No owner's words: a mend. The `agentik` door is one `sh` file every tool carries and installs at `~/.local/bin/agentik`; it reads `VERSION=0.1.0` today and gains a rule at 0.2.0. The suite's canonical install module writes the door forward only. This repo's `make install` copies its vendored door over whatever is there (`Makefile:50`, `install -m 0755 scripts/agentik $(BINDIR)/agentik`), so each install of the relay puts the old door back. This row lands before any 0.2.0 door is installed.

## 2. Rulings

1. **R1, one script.** `scripts/install-door SRC DEST`, POSIX `sh`, this repo's own: not vendored, no line in `scripts/vendored.sha256`. It uses `sed`, `awk`, `install` and `mv` and nothing a bare macOS or Linux lacks.
2. **R2, the rule.** Nothing at `DEST`: written. `DEST` a link: left alone. A `VERSION=` line, in `DEST` or in `SRC`, that is not three numbers with dots between: left alone. `SRC` newer, compared number by number (0.10.0 is above 0.9.0): written. Equal or newer at `DEST`: it stays, byte for byte. The first `VERSION=` line of a file is the one read; a prerelease (`0.2.0-rc.1`) reads as no version.
3. **R3, what is said.** Written or kept: no output. Left alone: one stdout line, `the door <DEST> is a link; left alone` or `the door <DEST> has no version to compare (<theirs> there, <ours> here); left alone`, `none` for a missing line. Exit 0 in every case of R2; exit 1 with one stderr line when `SRC` cannot be read or the write fails.
4. **R4, the write.** A copy at mode 0755 to a temp file beside `DEST`, then one `mv -f` over it; no temp file is left on a failure.
5. **R5, the Makefile.** Line 50 becomes `sh scripts/install-door scripts/agentik $(BINDIR)/agentik`; the target's `##` text says the door is placed where none or an older one is. `TestAgentikR1_5_InstallTarget` pins the new line; its other four stand.
6. **R6, the tests.** `cmd/agentik-relay/dw1_test.go`, `TestDW1_...`, each running `sh scripts/install-door` over made-up doors in `t.TempDir()`, the source the vendored `scripts/agentik` or a copy with its one `VERSION=` line changed. No test builds or runs a relay binary or `make install`.

## 3. Interfaces

`sh scripts/install-door SRC DEST`: exit 0 written, kept or left alone (R3's line on stdout for the last); exit 1, one stderr line, `SRC` unreadable or the write failed; exit 2, usage, not two arguments. Tests: `TestDW1_1_NewerKept`, `TestDW1_2_Written` (none, older), `TestDW1_3_NumberByNumber`, `TestDW1_4_LinkLeft`, `TestDW1_5_NoVersion` (door, source, prerelease), `TestDW1_6_SourceMissing`.

At the pin, line 50's command run by hand over made-up doors: a door at `VERSION=0.2.0` is overwritten to `0.1.0`; a link at `DEST` becomes a file, its target untouched; a missing `SRC` exits 71 with `install`'s own line. `make -n install`'s fourth line reads `install -m 0755 scripts/agentik <dir>/agentik`.

## 4. Findings that name this row

None. Not here: the 0.2.0 door or any edit of `scripts/agentik`; what `make install` does with the binary, the `relay` link or GOPATH's link; a doctor check of the door's version; the README's install lines (its sentence on `make install` stays true); a script shared with other repos. Nothing under `plugin/` changes: the plugin stays at 0.2.2.

## 4a. Review 1: the findings and the ruling

- F1, fix: a FIFO at `DEST` hung the script (`awk` blocks on its open). `DEST` is read only when `[ -f ]`; anything else there (a FIFO, a device, a socket, a folder) gets the no-version line, exit 0, never opened. `TestDW1_7_NotAFile`.
- F2, fix: a relative `DEST` starting with `-` was read as `mv`'s option, exit 1 and a temp file left. A relative `DEST` is taken as `./DEST`; R3's lines still name it as given. `TestDW1_8_DashName`.
- L1, fix: `echo` cut or split R3's one line over a value holding a backslash or `-n`. Every line goes through `printf '%s\n'`. `TestDW1_9_OneLine`.
- L2, built: R4's temp file and one rename are pinned by a shim `mv` on `PATH` that logs its call. `TestDW1_10_TempThenRename`.
- F3, not built: two runs at once can leave the older door; no lock. Filed in `NICE_TO_HAVE.md` for the door's probe over its three writers, with the review's other leads.

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or night record; transcripts.

## 6. Report

The branch and head sha; the gate's id and its line; each acceptance line at the pin and now; choices; leftovers.
