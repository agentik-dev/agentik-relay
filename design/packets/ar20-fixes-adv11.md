# Packet AR20: the gate and the merge read nothing the child can write (ADV11 highs and mediums, the owner's push word)

| item | value |
|---|---|
| from | `design/review-adv11.md` whole; `design/relay-r1.md` §3, §6, §8, §10, §11, §12, §15, §16; the code the review cites; §1's cross-repo quotes (never read another repo) |
| pin | agent-relay `main` after `packets-38-relay` (cut off `main`, no `base`); worktree `.claude/worktrees/ar20-fixes-adv11` |
| wall | this repo only. No push, `--amend`, rebase, device or network; every test path under `t.TempDir()`, never the real queue, ledger or state root; a hand `relay plan` only with `--rows`, `RELAY_MEASURE_DEPTH=1` and a temp `XDG_STATE_HOME`; `go test ./...` only inside `make check` |
| may touch | `internal/gate/`, `internal/effects/`, `internal/night/`, `internal/relay/`, `internal/packet/`, `internal/stream/`, `internal/hooks/`, `internal/queue/`, `cmd/relay/`, `plugin/hooks/hooks.json`, `design/relay-r1.md`, `history/sittings/*.md`, `history/sessions/NEXT` (removed if present). Nothing else |
| acceptance | `go test ./internal/gate ./internal/effects ./internal/night -run TestAR20_ -count=1 2>&1 \| grep -c FAIL` reads 0; `go test ./internal/packet ./internal/stream ./internal/hooks ./internal/queue -run TestAR20_ -count=1 2>&1 \| grep -c FAIL` reads 0; `go test ./cmd/relay -run TestAR20_ -count=1 2>&1 \| grep -c FAIL` reads 0; `grep -rhc '^func TestAR20_' internal cmd \| paste -sd+ - \| bc` reads 12 or more; `grep -c 'TestAR20_' design/relay-r1.md` reads 8 or more; `test -e history/sessions/NEXT; echo $?` reads 1 |
| budget | 80 turns, 130k context, `claude-opus-5-5`; one commit per act (0 to 7) plus the charter's; `make check` in the foreground, then commit in the same turn; checkpoint at 64 turns: land the act in flight, name the rest deferred; the charter is the first free `history/sittings/2026-09-24-<n>-ar20.md`, under 4,096 bytes with no home path (measure with `wc -c` and `grep -c "$HOME"` before the last commit), closed before the last commit |
| verify | `make check` |

## 1. What the source says (quoted)

review-adv11: "the gate and the merge read nothing the child can write. That means a hermetic git (no hooks, no repo config), merge shas held in memory, main recorded for every row, and a commit that must equal the verified tree"; "Fix before carrying example-app: 1 to 7, 9, 10 and 12". The session's review: "The relay's `packet.Unbounded` should be run on the same cells" (`caffeinate -i`, `stdbuf -oL`, `arch -arm64` and `xargs` before the suite; `go test -run . ./...`, `go test all`, `make readme`, `make install`). The decisions review: a builder who may touch `src/decisions/` or `tests/` "lets it edit the check". Respeak's ADV11-5: `5-hour limit reached ∙ resets 3pm`, `Weekly limit reached ∙ resets Mon 9am` and `API Error: 429 rate_limit_error` are account stops; "share one pattern with the relay's account stop". The owner, 2026-09-24 ~08:00: "You have permission to git push in those relay sessions".

## 2. Rulings

Record each choice in the charter, at most ONE fork. Each act's test reproduces its finding FIRST (red on `main`), then the fix.

0. **A hermetic git for the gate and the merge (ADV11-3, -5, -14, the -11 doors, -17's stderr red).** Every git call the gate and the merge make goes through one runner: `GIT_CONFIG_NOSYSTEM=1`, `-c core.hooksPath=/dev/null -c core.fsmonitor=false`, stderr kept. Stage records a digest (sha256 over the common dir's `config`, `hooks/` and `info/`, and the worktree's own `config.worktree` if present); the gate and the merge recompute it first, and a change is `unrun`, reason `git-state-moved`, and nothing merges (fail early). `Fresh` is built from the root, not the child's tree. The merge commit runs with `--no-verify` under the runner, and its tree must equal the tree `mergedVerify` judged, else the merge is undone (`reset --hard` to the recorded main) and the row is red, reason `merge-tree-differs`. `TestAR20_0_GateHermetic` (review test 1, plus a filter in `.git/config`).
1. **Main bound for every row; merges held in memory (ADV11-1, -2, -17's root).** `Staged.MainSha` is recorded at stage for every row, whatever its base. The expected main is `MainSha` advanced only by the relay's own merges, held in a map keyed by root inside the process; `expectedBase` never reads the ledger. A row staged by an earlier process gates green only when main still equals its `MainSha`. `merge` lines carry `root`. `TestAR20_1_MainBoundEveryRow` (review test 2).
2. **Every make spelling, and a harness list per repo (ADV11-4; the decisions handover, inside D2).** `GNUmakefile` and `makefile` join `HarnessPaths` and `GuardedPaths`; the overlay deletes a branch's copy that main lacks. The queue gains `harness:` (defaults and per row, a list of repo-relative paths, default `[Makefile, scripts]`, the make spellings always added); the overlay and the guard read it. decisions' own queue will name `src/decisions` and `tests`. `TestAR20_2_GNUmakefileCannotNeuter`, `TestAR20_2_HarnessListFromQueue`.
3. **Remotes bound by URL and time (ADV11-6, -7, -16, -17's buffer).** Stage records each remote's resolved URL (`git remote get-url --all`) next to its refs. The gate `ls-remote`s that literal URL under the runner with a 30 s context, its own process group and `ssh -o BatchMode=yes` when `GIT_SSH_COMMAND` is unset. It compares after the lines and again after `mergedVerify`, not only before. The lines run under `ChildEnv`'s pushurl deny. A line's output keeps its last 64 KiB. `TestAR20_3_VerifyLineCannotPush`, `TestAR20_3_LsRemoteTimesOut`.
4. **The nested guard is not keyed on the env (ADV11-9, a fork-storm door).** The pid file lives at a fixed path from `os/user.Current().HomeDir` (never `$HOME` or `XDG_STATE_HOME`); a relative state root is refused, exit 2. The hook refuses `go run` and `go build` of any path that `filepath.Clean`s to `cmd/relay`, and `relay resume` the same way as `run`. `TestAR20_4_NestedByEnv` (review test 3).
5. **Acceptance by allowlist (ADV11-10; the session's cells).** `packet.Unbounded` passes only `go test` with named packages (never `./...`, `all` or `$(`) and a `-run` that does not match every test (`.`, `^`, `.+`, `.*`, `''`), plus read-only verbs (`grep`, `test`, `wc`, `cut`, `sed -n`, `git log`/`show`/`diff`/`rev-parse`, `cat`, `paste`, `bc`, `tail`, `head`, `echo`); all else refused. Every cell quoted in §1 and the review's list are table rows. The stems the new check refuses (ar10, ar13, ar15, ar17a to ar17d, ar18) join `spent`; records are never rewritten. `TestAR20_5_AcceptanceAllowlist`, `TestAR20_5_SpentStems`.
6. **Push, the account stop and the Write door (the owner's word; ADV11-12; respeak ADV11-5; ADV11-11).** The `push` guard denies only when `RELAY_ROW` is set, as `ChildEnv` sets it, and relay-r1 §10 cites the owner's words of 2026-09-24 ~08:00. `TestAR20_6_PushAllowedOutsideSitting`, `TestAR20_6_PushDeniedInSitting`. The account stop: digits count only in an `API Error: NNN` prefix, needles match whole words, and `limit reached` counts only after `usage`, `5-hour`, `weekly` or `session`. The cells live in `internal/stream/testdata/account-cells.tsv` (text, reason), the one table respeak's next re-sync copies. `TestAR20_6_AccountCells`: `req_011CT429xYz`, `catalog in` and `context limit reached` stop nothing; respeak's three phrasings stop. The `PreToolUse` matcher adds `Write|Edit|MultiEdit|NotebookEdit`, denying a path under `.git/` or the state root inside a sitting. The guards stay advisory (§10 says so).
7. **The design.** §6, §3, §8 and §10 gain acts 0 to 6. §11's table names every `TestAR20_`. §12 records ADV11-21: published history is not rewritten, and hygiene covers the tree (the owner's words, ~08:00). §15 names ADV11-8 as a known limit until a sandbox. §16 names the TLA gaps from the review as unmodelled. No `Night.tla` change.

Respeak's ADV11-3 holds (`effects.StopLine` skips make's trailer). Deferred, named on the charter: ADV11-8 (sandbox), -13 (the merge needs exit 0), -15 (the session's hygiene), -18 to -20.

## 3. Interfaces (signatures, not prose)

- `effects.Git.Hermetic(dir string, args ...string) (stdout string, err error)`; `effects.GitStateDigest(root, worktree string) (string, error)`; `night.Staged.MainSha string`, `.StateDigest string`, `.RemoteURLs map[string][]string`; `queue.Row.Harness []string`; the reasons `git-state-moved`, `merge-tree-differs`, `main-moved`.

## 4. Findings that name this row

On `main` 74cfb98: ADV11-1 at `gate.go:258`, ADV11-2 at `gate.go:171-176`, ADV11-3 at `effects/git.go:200`, ADV11-4 at `gate.go:42,47`, ADV11-5 at `git.go:295`, ADV11-9 at `pidfile.go:68` and `main.go:1157`, ADV11-10 at `packet.go:178`, ADV11-12 at `stream/account.go:17-19`, the push guard at `guards.go:29`.

## 5. Forbidden reads

Every other repo; the real state root; any ledger but a fixture's; the packets before `ar13-*`; `.claude/worktrees/` but your own.

## 6. Report (capped)

The table: each acceptance line, its exit and last line. Bullets: per act the test's red line on `main`; what was left undone; the open fork. Last line: the branch and its head sha.
