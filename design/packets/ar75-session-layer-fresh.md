# Packet AR75: the session layer is read only while fresh

| item | value |
|---|---|
| from | `design/session-layer-r1.md` §2, §3 and §5 item 1 (revised after its refuter round); `NICE_TO_HAVE.md`'s section "A night takes a frozen session layer through tmux"; `internal/effects/config.go` at 1094791 |
| pin | agent-relay `main` at 1094791 plus this packet's own commit; worktree `.claude/worktrees/ar75-session-layer-fresh`, branch `ar75-session-layer-fresh` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/effects ./cmd/agentik-relay`, `go test ./internal/effects -count=1`, `go test ./cmd/agentik-relay -run 'Doctor\|AR75\|AgentikR1' -count=1`, `make doclint`, `make hygiene` and `make validate`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/effects/config.go`, `internal/effects/config_test.go` and `internal/effects/agentik_r1_test.go` (their session-file fixtures only), `internal/effects/ar75_test.go` (new), `cmd/agentik-relay/main.go` (doctor's config row only), `cmd/agentik-relay/ar75_test.go` (new), `design/relay-r1.md` (§9's configuration sentence and §11's test list), `design/session-layer-r1.md` (its Status line only), `NICE_TO_HAVE.md` (the section's heading only), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/effects -run '^TestAR75_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR75_'` reads 6; `go test ./cmd/agentik-relay -run '^TestAR75_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR75_'` reads 1; `go test ./internal/effects -run '^Test(LoadConfig\|AgentikR1_\|AR59_1_\|AR75_)' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- FAIL'` reads 0 |
| budget | 60 turns, 120k context, `claude-opus-5-5`; one act, one commit |
| verify | `make check` |

## 1. What the source says (quoted)

session-layer-r1 §2.1: "`LoadConfig` reads that list from the layer file and reads the layer only when the list names the relay's own repo file and its own local file (§2.2), each at the sha256 that file has now; a null value matches only a file that is still absent, and a file that cannot be read for any other reason counts as changed. Paths compare after `filepath.Clean` only … The other inputs (the profiles, the overlay) are not checked."

§2.2: "the `config.local.yaml` in the same folder as the repo file the relay picked (`.claude/agentik/`, else the old `.claude/claude-code-session/`) … with its `relay` section, else `agent-relay`, read as the repo's file is."

§2.3: "A stale layer, one resolved for another checkout …, one with no `provider.inputs` list, or one with a malformed list is skipped whole, never in part. It is not listed in `Sources`. `Warnings` gains one line naming the layer and why, and doctor's config row reports it as a warning, never a failure."

§2.4: "`$XDG_STATE_HOME` with a leading `~/` is expanded under `HOME`."

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar75.md` in doclint's five-part shape, opened at the act, closed before the commit, no home path. The act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is.

1. **Four layers.** `ConfigPaths` returns user, repo, local, then the session's when `$CLAUDE_CODE_SESSION_ID` names one. The repo file is picked as today (NewElseOld); the local file is `config.local.yaml` in the picked file's folder, never NewElseOld on its own. The local file is read exactly as the repo file is (its `relay` section, else `agent-relay`, c6's warning when both). The index test in `LoadConfig` (`i > 0`) still means "holds a section".
2. **The freshness check is one function** over the parsed session file, the relay's repo and local paths, and `FS` (read through `FS.ReadFile`, hashed with `crypto/sha256`; no `FS` change). It returns "" when the layer may be read, else the reason. The reasons, in this order: `no provider.inputs`; `malformed provider.inputs` (an item that is not a mapping with a scalar `path`, or a `sha256` that is neither `null` nor 64 lowercase hex digits); `resolved for another checkout: <relay repo path> not listed` (likewise for the local path); `<path> changed since the session resolved it`. A relative `path` is skipped before any of these. The strict reader returns a null as the string `null`, which means absent.
3. **A skipped layer** adds `<layer path>: not read: <reason>` to `Warnings` and nothing to `Sources`, and `LoadConfig` returns no error for it. A layer file that does not parse is still an error, as today.
4. **The state dir.** A `$XDG_STATE_HOME` that is `~` or starts `~/` is expanded under `HOME` before the session file's path is built.
5. **Doctor.** The config row's warning case reads `<n> files read, warnings: <…>` in place of `old names: <…>`; the warnings are the same list. It stays a warning (never `fail`).
6. **The fixtures.** The three tests that plant a session file give it a `provider` block whose `inputs` list the fixture's repo and local paths at their sha256 (null for an absent local), so they pass unchanged in what they assert. Write a small test helper for the block if two tests need it.
7. **The tests.** In `internal/effects/ar75_test.go`, on the fake `FS`: `TestAR75_1_FreshLayerRead` (the layer's `verify_seconds` wins; a profiles input and an overlay input at wrong hashes do not matter); `TestAR75_2_StaleLayerSkipped` (three cases: the repo file edited after resolve; a recorded-null local now present; the repo file unreadable through an `FS` whose `ReadFile` fails for it; each leaves the repo's value, the warning naming that path, the layer absent from `Sources`); `TestAR75_3_OtherCheckoutSkipped` (the inputs name `/w/.claude/agentik/config.yaml` while the relay reads `/r`); `TestAR75_4_UnprovenLayerSkipped` (no provider; a sha256 of `abc`; an item without `path`; a relative path alone is skipped and the layer read); `TestAR75_5_LocalLayer` (local over repo, session over local; an old-folder local beside a new-folder `config.yaml` is not read; the old folder's pair is read when the new folder has no `config.yaml`); `TestAR75_6_StateHomeTilde`. In `cmd/agentik-relay/ar75_test.go`, `TestAR75_7_DoctorWarns`: `run([]string{"doctor","--json"}, …)` in process under a temp `HOME`, `XDG_STATE_HOME` and session id with a stale layer: the config row is a warning whose text holds `not read:` and the exit is that of a warning. TestAR75_1 is the one red on the pin.
8. **relay-r1 §9.** The configuration sentence names the four layers, the local file's folder rule and the freshness rule in one added sentence citing session-layer-r1; §11 lists the seven tests. session-layer-r1's Status line gains "AR75 built §5 item 1". The NICE heading gains `(AR75)`.

## 3. Interfaces (signatures, not prose)

`func ConfigPaths(fs FS, env func(string) string, repo string) []string` keeps its signature and returns four paths with a session id, three without. One new unexported function, for example `func sessionFresh(fs FS, root yaml.Node, repoFile, localFile string) string`. No exported name changes; no ledger or `--json` key changes.

## 4. Findings that name this row

`NICE_TO_HAVE.md`'s "A night takes a frozen session layer through tmux" (the observer, 2026-10-14; S2). session-layer-r1 §7 notes 3, 4, 5 and 6.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory and the session plugin's state directory; the installed plugin cache. Read no other repo; the session plugin's resolver is quoted in session-layer-r1 where it matters.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s, `make doclint`'s and `make validate`'s exits, `make hygiene`'s last line. Bullets: TestAR75_1's red line on the pin; anything filed. Last line: branch, head sha.
