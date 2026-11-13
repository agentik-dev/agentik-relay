# Packet AK8: the relay answers `describe`, and each of its ten keys names the file and line that set it

| item | value |
|---|---|
| from | agentik-hq's row brief `ak8-plug-relay`; the owner's words below; the suite's plug contract (sections 2 to 7, held in §3 below) |
| pin | `main` at `fa29bcc` (`version` `0.1.0-dev`, plugin 0.2.2) plus this packet's own commit; branch `ak8-plug-relay`, worktree `.claude/worktrees/ak8-plug-relay`; nothing linked from main |
| wall | this repo's worktree only; no push, `--amend`, rebase, merge, install, plugin update; never `go build`, `go run`, `go install`, `make build` or `make install` by hand, never run a relay binary, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; `make check` only through the gate queue; no `DECISIONS.md` or rendered block edited; nothing under `internal/night` changed |
| may touch | `cmd/agentik-relay/describe.go` (new), `cmd/agentik-relay/main.go`, `internal/effects/config.go`, `cmd/agentik-relay/ak8_test.go` (new), `internal/effects/ak8_test.go` (new), `internal/night/testdata/json-keys.golden`, `plugin/commands/doctor.md`, `plugin/.claude-plugin/plugin.json`, `internal/packet/agentik_r1_test.go` and `internal/packet/p1_test.go` (their pins of the plugin's version and of `doctor.md`'s body), `NICE_TO_HAVE.md`, `design/packets/ak8-plug-relay.md`, `history/sittings/2026-11-13-2-ak8.md` |
| acceptance | `go test ./cmd/agentik-relay -run '^TestAK8_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAK8_'` reads `9` (main: `0`); `go test ./internal/effects -run '^TestAK8_' -count=1` exits 0 (main: 1, no such field); `go test ./cmd/agentik-relay -run '^TestJSONShapesFrozen$' -count=1` exits 0 (main: 0); `grep -c -F 'doctor: <n> failing' plugin/commands/doctor.md` reads `1` (main: `1`); `git diff main -- internal/night/status.go \| wc -l` reads `0` (main: `0`) |
| budget | 90 turns, 200k context, `claude-opus-5-5` at `high`; four acts, one commit each: 0 this packet; 1 the tests, red; 2 R4 in `config.go`; 3 R2 to R11, green, and the plugin page |
| verify | `make check` |

## 1. What the source says

The owner: "Make plugin configuration discoverable and drill down." The suite's `agentik config relay` lists this tool's settings and `agentik config relay.tmux` prints one as a card: the value, what it is, the layer, file and line that set it, each layer's value, and the lines to edit. At the pin `doctor`'s config check names the files read and the unknown keys, and nothing says which file set a key.

The ledger, verbatim:

> 7. **Notices keep quiet hours.** Notices keep quiet hours, not nights: the config names a quiet window (the owner's is 21:00 to 07:00, America/Los_Angeles), and a notice raised inside it goes to the log only. When the window ends and the run is still going, one catch-up notice gives the count of those held, never their text; outside the window, an unattended run notifies as it goes, whatever the hour it started. An orchestrated run refuses to start with no ntfy topic unless it is started with `--no-notify`, which keeps every notice of that run in its log. (2026-09-29; the owner's answer to decision 7, the owner's word in the selector, example-mac sitting 2026-09-28-6)

> 10. **The gate's verify time limit.** A repo's relay config may carry its own verify limit; when it is unset, the limit stays 900 s. `agentik relay doctor --verify` measures the repo's verify line and suggests a limit. A verify line cut at its limit reads red with a reason that says the line was cut, never with an empty reason. (2026-10-10; the owner's answer to decision 10, the owner's word in the observer session, 2026-10-10 15:37)

So `ntfy.topic`'s words say nothing that lets a run start without one, and `verify_seconds` reads 900 where no layer sets it.

## 2. Rulings

1. **R1, what stands.** `LoadConfig`'s signature and its answer for every input; every verb's text; every `--json` key that exists; `doctor`'s exit; the golden's `status`, `report`, `plan`, `run` and `gate` lines. New code prints "run", never "night".
2. **R2, the verb.** `describe` in `cmd/agentik-relay/describe.go`, one `case` in `run`, one line in `usage`. No pin, no git, make, sh or ps, no process, no write. The repo: the first folder up from the cwd holding `.git`; outside one there is no `repo` or `local` layer and no relative path is read. No `--json`, or a word after `describe` other than `config`: exit 2, one stderr line naming `describe --json`.
3. **R3, `describe --json`.** §3 below: `needs` `claude`, `git` (programs), `go` (toolchain), `tmux`, `caffeinate` (optional programs); `verbs` each verb of `usage` less `version`, `usage`'s own words (`clean` once, its two lines joined), `top` false; `reads` the config and doctor rows, then `status` named `run`, argv `status --json`.
4. **R4, the source of a key.** `Config` gains one exported field: each key `apply` met, with its layer's file, the line its value stands on and the value as written, lowest first, the keys it adds to `Unknown` among them. Nothing else in `config.go` changes meaning.
5. **R5, the layers.** `default`, `user` (travels), `repo`, `local`, `session` (only with `CLAUDE_CODE_SESSION_ID` set; present when the file was read).
6. **R6, the ten settings.** `tmux`, `allowed_tools`, `caffeinate`, `band`, `queue`, `merge`, `packets`, `verify_seconds`, `ntfy.url`, `ntfy.topic` (a secret, `"(set)"`); defaults read from the constants; `live` true; `change` an `edit` of the layer in force's file when it is `user`, `repo` or `local`, else the user's, the block at the top level there or under `relay:` in a repo file; a note on every key.
7. **R7, warnings and errors.** A row `unknown` per unknown key with its file and line; a row `choice` for a `tmux` value `TmuxMode` refuses. Every error of `LoadConfig`: §2's error, `bad_file`, exit 2.
8. **R9, `doctor --json`.** §5 below: `schema`, `tool`, `failed`, `warned` added, `ok` true when the read ran; each check gains `status` and `remedy` (only the queue's problems: `agentik relay plan`). Text and exit stand.
9. **R10, the plugin page** ends on `doctor: ok` when no check's `ok` is false; plugin 0.2.3.
10. **R11, `status --json`** opens `"schema": "agentik.relay.run/1", "ok": true`, by a struct embedding `night.Status`.

## 3. Interfaces

The plug contract, sections 2 to 7, cut to the rules this tool meets:

- **Envelope.** Every answer is one JSON object and a newline: `{"schema": "agentik.<read>/1", "ok": true, "tool": "relay", ...}`. A read that could not run prints `{"schema", "ok": false, "tool", "error": {"code", "message", "remedy"}}` on stdout, exit 2; `message` one line with the file and line.
- **`describe --json`**: `schema` `agentik.describe/1`, `ok`, `tool`, `version`, `summary` (one sentence under 80), `repo` (`<owner>/<repo>`), `plugin`, `needs` `[{name, kind, why, optional}]`, `verbs` `[{name, summary, top}]`, `reads` `[{kind, name, argv}]`.
- **`describe config --json [KEY]`**: `schema` `agentik.config/1`, `layers` `[{name, path, present, travels}]` lowest first; `settings` `[{key, value, default, type, choices, summary, source {layer, path, line}, change {kind, command, path, block, note}, live, values [{layer, value, path, line}]}]`; `warnings` `[{key, reason, value, path, line}]`. With KEY: that one setting, or `[]`, exit 0.
- **`doctor --json`**: `schema` `agentik.doctor/1`, `ok`, `tool`, `failed`, `warned`, `checks` `[{name, ok, warn, detail, status, remedy}]`.
- **A status read** gains `schema` and `ok` as its first two keys and keeps every other.
- **Tests**: `describe --json` exit 0, every key, no ESC; each read prints one object with `schema` and `ok`; the config read in a temp HOME; a user file's key at its line; an unknown key's warning; a bad file's `bad_file`.

At the pin, measured by reading the code (no relay verb run): `run` has no `describe` case; `doctor --json` prints `{checks: [{name, ok, warn, detail}], ok}`; `status --json` holds no `schema`.

## 4. Findings that name this row

Build 1: two tests pin what R10 changes, the plugin's version (`TestAgentikR1_5_PluginIsRelay`) and `doctor.md`'s body sha (`TestP1_1_CommandsHidden`); each pin moves to the new bytes, and nothing else in them.

## 5. Forbidden reads

Anything under the home's `.local/state` or `.claude/projects`; a relay ledger or run record; transcripts.

## 6. Report

The branch and head sha; the gate's id and its line; each acceptance line at the pin and now; choices; leftovers.
