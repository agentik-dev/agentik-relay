# Packet FH7: the README opens in the ten-line form, installs its binary where its second line finds it, and drops "night" and "morning"

| item | value |
|---|---|
| from | agentik-hq's row brief `fh7-relay-top`; the owner's words below |
| pin | `main` at `c98b17b` (plugin 0.2.3) plus this packet's own commit; branch `fh7-relay-top`, worktree `.claude/worktrees/fh7-relay-top`; nothing linked from main (a Go repo, no venv) |
| wall | this repo's worktree only; no push, `--amend`, rebase, merge, install, plugin update; never `go build`, `go run`, `go install` or `make install` by hand, never run a relay binary, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; no `claude` process, no render; `make check` only through the gate queue; no `DECISIONS.md` or rendered block edited |
| may touch | `README.md`, `design/readme/source.md`, `design/readme/rendered.sha256`, `design/packets/fh7-relay-top.md`; nothing under `plugin/`, `cmd/` or `internal/` |
| acceptance | `sed -n '1,/^## /p' README.md \| grep -c -v -e '^## ' -e '^$' -e '^<img '` reads `10` (main: `2`), the top's eight counted lines and its fence's two marks; `sed -n '1,/^## /p' README.md \| grep -c '^[^[:alnum:][:space:]#<~]'` reads `2` (main: `0`), the fence's two marks, one fence; `sed -n '1,/^## /p' design/readme/source.md \| grep -c -v -e '^## ' -e '^$' -e '^<img '` reads `10` (main: `2`); `sed -n '1,/^## /p' design/readme/source.md \| grep -c '^[^[:alnum:][:space:]#<~]'` reads `2` (main: `0`); `grep -ci -e night -e morning README.md` reads `0` (main: `17`); `grep -ci -e night -e morning design/readme/source.md` reads `0` (main: `17`); `grep -c -F 'GOBIN="$HOME/.local/bin" go install' README.md` reads `2` (main: `0`); `grep -c -F 'GOBIN="$HOME/.local/bin" go install' design/readme/source.md` reads `2` (main: `0`); `grep -c '^go install' README.md` reads `0` (main: `1`); `grep -c '^go install' design/readme/source.md` reads `0` (main: `1`), and the sidecar's `shasum -a 256 -c design/readme/rendered.sha256`, `scripts/doclint`, `scripts/hygiene` and `scripts/plugin-version-check` exit 0 inside the verify row's `make check`, whose `check-unlocked` runs each (none is on the acceptance allowlist) |
| budget | one sitting, words only; two commits: this packet, then the change |
| verify | `make check` |

## 1. What the source says

The owner: "How close are we to being able to install this on my work computer?" He will install the suite on a second computer from the public repos alone, and strangers will after him. A newcomer reads the first ten lines of a README and decides. Today this one's first lines speak to its maintainers, and its install is below the first screen.

The ledger's own words bind and stay: "night" stays in the ledger and in the relay's own output. The README names no entry by its title.

## 2. Rulings

1. **R1, the top.** Everything before the first `##`, in `README.md` and `design/readme/source.md`, becomes one text, the same bytes in both: the title, the icon line, one sentence on what a person gets, one sentence starting "Use it when", one sentence saying where the block is typed, one fenced block of two lines (the install, then `~/.local/bin/agentik-relay doctor`), one line of needs, one line linking the organisation's page. Eight counted lines, one fence. The second line runs where the first leaves the person; the install joins its verbs with `;` so the second verb runs where the marketplace is known already.
2. **R2, nothing is lost.** The old lead's facts (a static Go binary with `relay` a link to it until 2026-12-31; a plugin, `relay`; one worktree, one branch and one session a row; a row merges into `main` only when its gate reads green) become the first paragraph under `## Install`.
3. **R3, by hand.** Each change is made in both files; no render. The sidecar is the `readme-stamp` recipe's own line.
4. **R7, the install line below the top.** In the `## Install` fence the third line becomes `GOBIN="$HOME/.local/bin" go install github.com/agentik-dev/agentik-relay/cmd/agentik-relay@latest`, its first two lines standing; one bullet under the fence says the binary lands beside every other agentik command, so one PATH line serves them all.
5. **R8, the words.** "night", "overnight" and "morning" leave both files: a night is "a run", "overnight" is "unattended", "Morning:" is "After a run:", `## The night` is `## A run`. A sentence that leaned on the time of day says what it meant: nobody watches, and one report is read when the run ends.

## 3. Interfaces

No code, no verb, no flag, no file format. `go env GOBIN` with `GOBIN="$HOME/.local/bin"` given prints that folder, which is where the second line looks; with none given `go install` writes `~/go/bin`, on no newcomer's PATH. Whether `agentik-relay doctor` exits 0 on a new machine is not read here.

## 4. Findings that name this row

None yet. Not built:

- The Status line, which trails the plugin's version.
- Anything under `plugin/`, `cmd/` or `internal/`: the plugin's commands each say `agentik-relay is not on PATH: make install`, a lead for a later plugin version.
- The relay's own output and its ledger, which say "night".

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or run record; transcripts.

## 6. Report

The branch and head sha; the gate's id and its line; each acceptance line at the pin and now; where each of R2's facts now stands; choices; leftovers.
