# Packet RV1: the binary says the plugin's version; the relay knows session's `enabled` key

| item | value |
|---|---|
| from | agentik-hq's row brief `rv1-binary-version`, the owner's words below; two confirmed leads of 2026-11-20 |
| pin | `main` at `b281ef7` (plugin 0.2.5) plus this packet's own commit; branch `rv1-binary-version`, worktree `.claude/worktrees/rv1-binary-version`; nothing linked from main (a Go repo, no venv) |
| wall | this repo's worktree only; no push, `--amend`, rebase, merge, install, plugin update; never build a relay binary by hand, never run one outside `make`'s test harness (so `go test ./cmd/agentik-relay` and R2's target run only inside the gate's check), never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; no `claude`; `make check` only through the gate queue; no `DECISIONS.md` or rendered block edited |
| may touch | `cmd/agentik-relay/main.go`, `cmd/agentik-relay/describe.go`, `cmd/agentik-relay/rv1_test.go` (new), `cmd/agentik-relay/main_test.go` (`TestVerbs`' version line, `TestDoctorTable`'s names and letters), `cmd/agentik-relay/ak8_test.go` (the settings' count), `cmd/agentik-relay/agentik_r1_test.go` (`TestAgentikR1_5_InstallTarget`'s install and build lines), `internal/effects/config.go`, `internal/effects/rv1_test.go` (new), `internal/packet/agentik_r1_test.go` (the plugin's version pin), `internal/packet/p1_test.go` (`doctor.md`'s body sha), `Makefile`, `plugin/.claude-plugin/plugin.json`, `plugin/commands/doctor.md`, `README.md`, `design/readme/source.md`, `design/readme/rendered.sha256`, `design/packets/rv1-binary-version.md`, `design/packets/rr6-relay-records.md` (its review's words), `NICE_TO_HAVE.md`, `history/sittings/2026-11-20-2-rv1-binary-version.md` |
| acceptance | `go test ./internal/effects -run '^TestRV1_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestRV1_'` reads `3` (main: `0`); `go test ./cmd/agentik-relay -run '^TestRV1_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestRV1_'` reads `4` (main: `0`), run inside the verify row's check only; `grep -c -F 'main.version=' Makefile` reads `1` (main: `0`); `grep -c -F '"version": "0.2.7"' plugin/.claude-plugin/plugin.json` reads `1` (main: `0`); `grep -c -F -- '--plugin-root' plugin/commands/doctor.md` reads `1` (main: `0`) |
| budget | 90 turns, 200k context, `claude-opus-5-5`; five acts, one commit each: 0 this packet; 1 the tests, red; 2 R1 to R4, green; 3 R5; 4 rr6's review in words |
| verify | `make check` |

## 1. What the source says

The owner, 2026-11-20: "continue with the charter", and, giving the sitting six workers, "to
accelerate your work". Both findings are an advisor's leads of that day, each confirmed by
reading: the binary's version is the constant `0.1.0-dev` while the plugin is 0.2.5 and neither
build line stamps it, so nothing can tell a stale binary from a current one; and every doctor
warns that the relay does not know `enabled`, a key of the `relay` section that the session tool
writes and reads. This repo's `DECISIONS.md` binds; no entry of it changes.

## 2. Rulings

1. **R1, the stamp.** `version` is a variable whose unstamped value is `unstamped`. The
   Makefile's `build` and `install` lines stamp it with the plugin's version, read from
   `plugin/.claude-plugin/plugin.json` by the Makefile's own read (the one `readme-status` used),
   through `-ldflags "-X main.version=..."`. `version`, `--version` and `describe` print it;
   `go run`, `go test` and a bare build print `unstamped`, and nothing fails on it.
2. **R2, the check.** `make check` runs a target that builds into a temp folder with the
   Makefile's own flags, asserts `--version` prints `relay <the plugin's version>`, and removes
   the folder.
3. **R3, the doctor** (as review 1's F2 and F3 rule it). One row, `binary`, before `template`,
   against `<root>/.claude-plugin/plugin.json`, the root `doctor --plugin-root <dir>`'s, else
   `$CLAUDE_PLUGIN_ROOT`'s; `/relay:doctor` passes `--plugin-root "${CLAUDE_PLUGIN_ROOT}"`, since
   the Bash tool's shell holds no such variable. An empty flag reads as none given. `ok`, `relay V,
   the plugin's` where the versions are equal; `warn`, `relay V, the plugin P: reinstall the
   binary (make install in a clone, or the README's go install line)`, remedy `make install`,
   where a stamped V differs; `ok`, `relay unstamped, the plugin P: a go install build carries no
   version; make install from a clone stamps it`, no remedy, where V is `unstamped`; `ok`, `relay
   V` alone when no root is known or the file does not read or holds no version. Text and JSON.
4. **R4, `enabled`.** The `relay` section's `enabled` is known: read by the session tool, by no
   behaviour here. A scalar among `auto`, `on`, `off` and the YAML bools the session's reader
   takes (`true`, `True`, `TRUE`, `false`, `False`, `FALSE`; review 1, F1) is known, kept in
   `Config.Enabled` and shown by `describe config` with one line saying whose it is; any other
   value stays in `Unknown` exactly as at the pin (`<file>: enabled`), as does a misspelt key.
   The warning for the two sections present together stands.
5. **R5, what stands.** Every verb's output but the version string and the two doctor lines;
   the hooks; the guards. The README's version clause in both files and the sidecar; the plugin
   at 0.2.7 with its version pin (0.2.6 in build 1; `doctor.md` changed in build 2), the last
   commit under `plugin/`. The README's install line says `make install` stamps the plugin's
   version.

## 3. Interfaces

`var version = "unstamped"` (`cmd/agentik-relay/main.go`). `doctor(asJSON bool, pluginRoot
string, stdout io.Writer) int`. `pluginRootOf(flag string, getenv) string`: the flag, else the
variable. `pluginVersion(root string) string`: the plugin file's version, "" when unset or unread. `binaryRow(bin, plugin string) (ok, warn bool,
detail string, remedy *string)`. Makefile: `PLUGIN_VERSION`, `STAMP`, target
`binary-version-check` in `check-unlocked`. `effects.Config.Enabled string`; describe's
eleventh setting `enabled`.

## 4. Findings that name this row

Review 1: F1 to F3 and F5 built in build 2; F4 and F7 are NICE n29 and n30. Not built: a binary row for the `agentik` front door's own version.

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or run record;
transcripts; the coordinating repo's row file.

## 6. Report

The branch and head sha; the gate's id and its line; each acceptance line at the pin and now;
the tests that pinned the constant or the counts; choices; leftovers.
