# Packet AR89: the helper guard holds F3's four evasions and their twins

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`: S3 ("put AR49's four helper evasions … in the guard's test table, expected denied, with F3's fix (item 11)") and the AR40 review's F3; `design/guard-parser-r1.md`, "Why a parser at all"; the coordinator's rulings (2026-10-16) |
| pin | `main` at 6fd429b plus this packet's own commit; a linked worktree, branch `ar89-guard-evasions` |
| wall | this repo only. No push, `--amend`, rebase, merge, `git stash`, device, ssh or network. Never build a `relay` binary by hand or run one; `go test` on packages is the harness. No `relay` or `agentik relay` verb in any shell: the evasion strings live only in Go test tables. No `pkill -f`. No make target but `make doclint`. Never write in the account's state directory. Hygiene's `lan` rule matches the Go name of the local zone: never spell it. |
| may touch | `internal/hooks/guards.go` (`helperRelay`, its two new unexported readers, the `HelperRelay` comment), `internal/hooks/ar89_test.go` (new), `design/guard-parser-r1.md` ("Why a parser at all"'s last sentence), `history/sittings/2026-10-16-1-ar89.md` (new), this packet |
| acceptance | `go test ./internal/hooks -run '^TestAR89_' -count=1 -timeout 120s -v 2>&1 \| grep -c -E '^--- PASS: TestAR89_'` reads 4; `go test ./internal/hooks -run '^Test(AR\|AgentikR1\|PublicR1\|Match\|GuardsWithout)' -count=1 -timeout 300s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0 |
| budget | 45 turns, 180k context, `claude-opus-5-5`; stop at 38 turns; three acts (the packet, the table and its fix, the record), one commit each |
| verify | `make check` |

## 1. What the source says (quoted)

NICE, the AR40 review's F3: "`$(which relay) stop`, `` `which relay` stop ``, `r=relay; $r stop` and `pkill -f 'relay run'` pass from a helper. Give helpers RowShapes' `literal` guard."

NICE, S3: "No test in `internal/hooks` holds them at 9f1d9e5; 'Configure permissions' (as of 2026-10-14) says a Bash deny rule is not a security boundary, so the guard is the only reader of these spellings."

guard-parser-r1, "Why a parser at all": the four "belong in the guard's test table as rows, denied once F3 lands; no test holds them at this head."

The pin: `bashDoor` (`internal/hooks/guards.go`) adds `HelperRelay` and `unreadableBound` to `guards(env)` for a subagent's call; `helperRelay` reads only a binary `relayBin` can name. Out of a row a helper meets no `literal` guard, so a hidden command word (`$()` once the reader has split the substitution off, `$r`, `${…}`) names no binary and passes. In a row `RowShapes`' `literal` denies the hidden words first; nothing reads `pkill` or `killall`.

## 2. Rulings

1. **The rows.** Each of the four, and its twin under `agentik relay` and `agentik-relay`, is a row of `ar89Evasions` in `internal/hooks/ar89_test.go`, expected denied for a helper (a call whose payload carries `agent_id` or `agent_type`, as `helperSays` builds it): `helper-relay` out of a row, any deny in one. The table adds the other writing verbs and quoted spellings of the same shapes.
2. **A hidden command word.** `helperRelay` also denies a segment whose command word is not literal (`literal`), or whose word after `agentik` is not, when a word after it is one of `helperVerbs`: the binary is then any binary, so a verb that writes a night is read as the relay's. A hidden word with no such verb passes (`$(which relay) status`, `$EDITOR file`). Not the whole `literal` guard, F3's wording: out of a row it would deny a helper's `"$(go env GOPATH)/bin/staticcheck" ./...`, which names no relay. A row's helper keeps the `literal` guard it has.
3. **A signal by name.** `helperRelay` also denies a `pkill` or `killall` segment one of whose operands (a word not starting with `-`) contains `relay`: the pattern may match a night's `relay run`, `agentik relay run` or `agentik-relay run`. `pgrep` and `ps` pass.
4. **The false positives.** `ar89Passes` holds the read verbs (`relay status`, `relay report` without `--write`, their `agentik relay` and hidden-name spellings), `which relay`, a grep, an rg, a cat and a git log of relay paths, `pgrep -f 'relay run'`, a `pkill` of a go test, and hidden command words with no write verb: none is denied for a helper.
5. **Unchanged.** `HelperRelay`'s name and reason, the main thread's verdicts (no `agent_id`, no `agent_type`, out of a row: no guard), `nestedRun`, `RowShapes` and every existing test.
6. **The record.** guard-parser-r1's last sentence of "Why a parser at all" reads that the four and their twins are rows of AR89's table, denied for a helper.

## 3. Interfaces (signatures, not prose)

`hooks`, unexported: `func hiddenWrite(words []string) bool`, `func relaySignal(words []string) bool`; `helperRelay` returns `hiddenWrite(words) || relaySignal(words)` where `relayBin` finds no binary. No exported name, guard, reason or golden changes.

## 4. Findings that name this row

NICE's F3 (the AR40 review) and S3; the coordinator marks both. What AR89 leaves: `kill $(pgrep -f 'relay run')` (the pid comes from a read), a `pkill` pattern that matches the night without the word (`'r.lay run'`), and a write verb that is itself hidden (`$r $v`); the guards are advisory (relay-r1 §10).

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory and the session plugin's. Read no other repo. `NICE_TO_HAVE.md` by grep only, never edited here.

## 6. Report (capped)

At most 120 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits. Bullets: the rows red on the pin; any ruling beyond §2; the head sha.
