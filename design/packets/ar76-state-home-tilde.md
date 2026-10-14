# Packet AR76: one state home, `~` expanded; the moved folder; a null list

| item | value |
|---|---|
| from | `design/session-layer-r1.md` §2.3, §2.4 and §6; three notes of AR75's review round; `internal/effects/effects.go`, `internal/effects/config.go` and `internal/hooks/hooks.go` at 98fa355 |
| pin | agent-relay branch `ar75-session-layer-fresh` at 98fa355 (AR75, reviewed, not yet merged) plus this packet's own commit; worktree `.claude/worktrees/ar76-state-home-tilde`, branch `ar76-state-home-tilde` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `go test`'s harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make install`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines, `go vet ./internal/xdg ./internal/effects ./internal/hooks`, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/xdg/xdg.go` (new), `internal/xdg/ar76_test.go` (new), `internal/effects/effects.go` (`StateDir` only), `internal/effects/config.go` (`ConfigPaths`' state base, `sessionFresh` and its helpers), `internal/effects/ar76_test.go` (new), `internal/hooks/hooks.go` (`stateBase` and its comments only), `internal/hooks/ar76_test.go` (new), `design/relay-r1.md` (§9's configuration sentence and §11's test list), `design/session-layer-r1.md` (its Status line and §6's cost line only), `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/effects -run '^TestAR76_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR76_'` reads 3; `go test ./internal/hooks -run '^TestAR76_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR76_'` reads 1; `go test ./internal/xdg -run '^TestAR76_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR76_'` reads 1; `go test ./internal/effects -run '^Test(StateDir\|LoadConfig\|AgentikR1_\|AR59_1_\|AR75_\|AR76_)' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- FAIL'` reads 0; `go test ./internal/hooks -run '^Test(Run\|AgentikR1_\|AR20_6_\|AR21_2_\|AR23_\|AR27B_\|AR28_\|AR76_)' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- FAIL'` reads 0 |
| budget | 60 turns, 120k context, `claude-opus-5-5`; one act, one commit |
| verify | `make check` |

## 1. What the source says (quoted)

session-layer-r1 §2.4: "`$XDG_STATE_HOME` with a leading `~/` is expanded under `HOME`, as the session plugin expands it, so both name one state dir." AR75 did it for the session file only (`config.go:98-104`). `StateDir` (`effects.go:184-190`) and the hooks' `stateBase` (`hooks.go:168-178`) still take `~/x` as written: a relative state root, which `holdPid` refuses, and a file door that guards no state root (`fileDoor`'s `under` wants an absolute root).

§2.3: "A stale layer, one resolved for another checkout …, one with no `provider.inputs` list, or one with a malformed list is skipped whole". `sessionFresh` (`config.go:182-225`) names a layer listing the relay's repo file under the other config folder `resolved for another checkout`, though it is this checkout before the folder moved; and `provider: {inputs: null}` reads `malformed provider.inputs`, though YAML's null is no list.

§6: "**Cost.** Each `LoadConfig` hashes two small files, on every verb and every `status --watch` tick. Not measured; small." `status --watch` goes to `night.Watch` (`cmd/agentik-relay/main.go:1418`) and never calls `LoadConfig`; AR75's review measured one `LoadConfig` with a fresh layer at about 57 µs.

## 2. Rulings

Standing rules. The charter is `history/sittings/<the build date>-<n>-ar76.md` in doclint's five-part shape, opened at the act, closed before the commit, no home path. The act's first test is red on the pin; its red line goes in the commit message. `DECISIONS.md` stays as it is.

1. **One state home.** A new package `internal/xdg`, stdlib only, holds the one helper `StateHome`: `$XDG_STATE_HOME`, else `$HOME/.local/state`; a value of `~` or one starting `~/` read under `HOME`; any other value as written (`~x`, a relative path); "" when the value needs `HOME` and `HOME` is empty. `StateDir` is `StateIn(xdg.StateHome(env))`; `ConfigPaths` builds the session file under `xdg.StateHome(env)` (its own switch goes); the hooks' `stateBase` is `xdg.StateHome(env)`. The hooks import nothing else of the relay, and `hooks.go`'s comment says so.
2. **The moved folder.** When the relay's repo or local path is not listed but its other-folder twin is (the same file name in the repo's other config folder, `.claude/agentik/` for `.claude/claude-code-session/` and back, by `config.go`'s `repoConfig` and `oldRepoConfig`), the reason reads `resolved before the config folder moved: <the listed twin>`. Any other unlisted path keeps `resolved for another checkout: <path> not listed`. The reasons' order is otherwise AR75's.
3. **A null list.** An `inputs` that is YAML's null (`null`, `Null`, `NULL`, `~`, or no value) reads `no provider.inputs`, as an absent key does. Any other scalar stays `malformed provider.inputs`; `[]` stays a list that lists nothing.
4. **session-layer-r1.** §6's cost line reads that each `LoadConfig` hashes two small files, once per verb that loads the configuration; that `status --watch` never calls it (`main.go:1418`, `night.Watch`); and that one `LoadConfig` with a fresh layer measured about 57 µs. The Status line gains that AR76 took three notes of AR75's review.
5. **relay-r1.** §9's configuration sentence gains one sentence: the relay's state dir and the hooks' state path read `$XDG_STATE_HOME` through the one helper, `~` expanded likewise, and the two reasons of rulings 2 and 3. §11 lists the five tests after AR75's.
6. **The tests.** `internal/effects/ar76_test.go`: `TestAR76_1_StateDirTilde` (under `HOME=/h`, `XDG_STATE_HOME` `~/x`, `~`, `/s`, `~x` and unset give `StateDir` `/h/x`, `/h`, `/s`, `~x` and `/h/.local/state`, each joined with `agentik-relay`, and `ConfigPaths`' session file lies under the same base); `TestAR76_2_FolderMovedReason` (the relay reads `/r`'s new folder, the layer lists the old folder's pair: the moved reason naming the old `config.yaml`; the relay reads the old pair, only it existing, the layer lists the new folder's: the reason names the new one; `/w`'s files still read `another checkout`); `TestAR76_3_NullInputs` (`provider: {inputs: null}`, `inputs: ~`, `inputs: NULL` and a bare `inputs:` read `no provider.inputs`; `inputs: none` stays malformed; `inputs: []` reads `another checkout`). `internal/hooks/ar76_test.go`: `TestAR76_4_HookStateTilde` (`XDG_STATE_HOME=~/x` under a temp `HOME`: `statePath` and `SoftPath` under `<home>/x/agentik-relay/sessions/s1/`; in a row with `PidDir` nil the file door denies `<home>/x/agentik-relay/ledger/a.jsonl` as `state-root`; `HOME` and the value both empty, `statePath` ""). `internal/xdg/ar76_test.go`: `TestAR76_5_StateHome` (the helper's table, `HOME` set and empty). Old names are spelled through `config.go`'s constants, so no test needs a `.hygiene-allow` entry. TestAR76_1 is the one red on the pin.

## 3. Interfaces (signatures, not prose)

`package xdg`: `func StateHome(env func(string) string) string`. In `internal/effects`, unexported helpers only, for example `func twin(p string) string` and `func null(n yaml.Node) bool`. `StateDir`, `ConfigPaths` and `LoadConfig` keep their signatures. No other exported name changes; no ledger or `--json` key changes.

## 4. Findings that name this row

AR75's review round, three notes: `~` in `$XDG_STATE_HOME` for the state dir and the hooks; the moved folder's reason; a null `inputs`. session-layer-r1 §6's cost line.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory and the session plugin's state directory; the installed plugin cache. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line's count, `go vet`'s and `make doclint`'s exits, `make hygiene`'s last line. Bullets: TestAR76_1's red line on the pin; anything filed. Last line: branch, head sha.
