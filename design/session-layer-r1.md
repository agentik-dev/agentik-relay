# agent-relay design: the session layer is read only while fresh (r1)

**Status:** design r1, 2026-10-14, written before code; one refuter round to come (§7). It amends relay-r1 §9 (configuration). The settled entry 10 (the repo's verify limit) is what this record makes reach a night; no entry of `DECISIONS.md` covers the session layer. The finding is `NICE_TO_HAVE.md`'s "A night takes a frozen session layer through tmux".

## 1. The problem

`ConfigPaths` (`internal/effects/config.go:73`) reads three layers, lowest first: the user's file, the repo's `.claude/agentik/config.yaml`, and, when `$CLAUDE_CODE_SESSION_ID` names a session, that session's `resolved.yaml`, written by the session plugin at its SessionStart and on `set`. The session plugin writes the file's `relay` section from the shipped and the user's profiles, the repo's `config.yaml`, its `config.local.yaml` and the launch profile (`claude_session/resolver.py:396-405`). Its overlay (`set`) feeds the `session` section only (`:407-415`). So the `relay` section is a copy of files, taken when the session started, and it sits above the repo's own layer.

A night started by a chain script in tmux inherits the id of the sitting that started it. On 2026-10-14 at 01:39 the consumer raised `verify_seconds` from 1800 to 3600 on main. The sitting that started night 2026-10-14-1 at 03:42 had resolved at 23:13 the day before, with 1800. The night cut a row's verify at 1800 (ledger `"cut":1800`, red `verify-cut`), and a hand gate went green in 17 minutes. Any night or verb run under a session older than a config edit reads the old values.

## 2. The decision

1. **The session layer is read only while fresh.** Conventions §3 has each resolved file list its inputs, `provider.inputs: [{path, sha256}]`, with `sha256` null for a file that was absent. `LoadConfig` reads that list from the layer file and reads the layer only when, for every entry whose `path` is absolute, the file's sha256 now equals the recorded value, and a null value matches only a file that is still absent. A file that cannot be read for any other reason counts as changed. An entry whose path is not absolute names no file and is not checked; the plugin writes none today.
2. **A layer that cannot be shown fresh is not read.** A stale layer, one with no `provider.inputs` list, or one with a malformed list is skipped whole, never in part. It is not listed in `Sources`. `Warnings` gains one line naming the layer and the first changed input, and doctor's config row reports it as a warning, never a failure.
3. **The relay reads `config.local.yaml` itself.** It is the third layer, between the repo's file and the session's: `.claude/agentik/config.local.yaml`, else the old `.claude/claude-code-session/config.local.yaml` (NewElseOld), with its `relay` section, else `agent-relay`, read as the repo's file is. Conventions §3 already lists this layer, and relay-r1 §9 leaves it out. Today it reaches the relay only through the session layer, so without it a skipped layer would drop a local value.

With the layer fresh, every value is what it is today. With it stale, the relay reads the same files itself, so only a profile's own `relay` keys are lost. The shipped profiles carry only `enabled`, `queue` and `merge` at the relay's own defaults, and the relay never reads `enabled`.

**Rejected.** A night's verbs skip the session layer: a hand `gate` from an older sitting is as stale, and the profile's keys would differ by verb. The session plugin writes only a session's own overrides: that changes conventions §3 for four plugins, and since the overlay cannot set relay keys, the relay's section would always be empty.

## 3. The seam and the record

- `internal/effects/config.go`: `ConfigPaths` gains the local layer. One function decides whether the session layer is fresh, given the parsed file and `FS`. If the strict YAML reader cannot read the `inputs` list (a flow mapping per item), it reads the `resolved.json` beside the file. `FS` gains a read-and-hash only if it has none.
- No ledger key and no `--json` key changes. A night records nothing new: the repo's value wins, and the ledger's `cut` already says which limit ran.
- relay-r1 §9's configuration sentence names the four layers and the freshness rule.

## 4. The proof

**P1, the incident, 2026-10-14 05:50.** The stale session's `resolved.json` lists the consumer's `config.yaml` at sha256 `59865fd0…`; the file on main now hashes `f62b042b…` and holds `verify_seconds: 3600`. Under §2 the layer is skipped and the repo's layer gives 3600.

**P2, the census, the same minute.** Of the 779 `resolved.json` files in the session plugin's state dir, all list `provider.inputs`. 154 are fresh and 625 are stale. In 428 of the stale ones, a repo's `config.yaml` changed. The rest changed only through a shipped `profiles.yaml`, whose path names the plugin's install tree, which an install replaces. Most are ended sessions, and any of them is the layer of a night its sitting starts.

## 5. The cut into packets

1. **AR75, the fresh session layer.** §2 and §3. Acceptance by named tests in `internal/effects`: a session layer whose inputs all hash as recorded is read (its `verify_seconds` wins over the repo's); one input changed, or a recorded-null input now present, or an input unreadable, leaves the layer unread, the repo's value in force, a warning naming that input, and the layer absent from `Sources`; no `provider` block, or a malformed `inputs`, leaves it unread with a warning; a relative path in `inputs` is not checked; `config.local.yaml` (and the old path) is read between the repo's file and the session's; doctor's config row shows the stale warning and does not fail. The existing tests that plant a session file give it a `provider.inputs` block.

## 6. Open risks

- **Install churn.** An install of the session plugin makes every older session's layer stale, so the relay stops reading it. Nothing is lost while profiles carry no relay keys of their own.
- **A running relay keeps what it loaded.** A night started before an edit runs on its start values, as it does for the repo's layer. Unchanged here.
- **Cost.** Each `LoadConfig` hashes the listed inputs, five small files today, on every verb and every `status --watch` tick. Not measured; small.
- **The other plugins.** decisions and respeak read the session layer under the same §3 and freeze the same way. One sentence in conventions §3 would bind them too. It is filed with the session plugin, not taken here.
