# Packet AR53: git's own housekeeping does not move the git state

| item | value |
|---|---|
| from | the ledger of night 2026-10-02-4 (§1); `NICE_TO_HAVE.md`, the item under "The observer's check-in 32" |
| pin | agent-relay `main` at the commit that lands this packet; worktree `.claude/worktrees/ar53-git-housekeeping`, branch `ar53-git-housekeeping` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state. No `make check` and no `go test ./...`: the acceptance lines, `go vet ./internal/effects ./internal/gate ./internal/night`, `make doclint` and `make hygiene` only, one at a time. **Python:** every shell line carries `PYTHONDONTWRITEBYTECODE=1`. A test that runs the real `git` does so only in its own `t.TempDir()`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/effects/git.go` (`GitStateDigest`, `digestTree` and what they need), `internal/effects/ar53_test.go` (new), `internal/gate/gate.go` (the git-state-moved detail only), `internal/gate/ar53_test.go` (new), `internal/night/stage.go` (where the binding is kept, for ruling 2 only), `design/relay-r1.md` (the git-state sentence), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/effects -run "^TestAR53_" -count=1 -timeout 300s -v 2>&1 \| grep -c "^--- PASS: TestAR53_"` reads 4; `go test ./internal/gate -run "^TestAR53_" -count=1 -timeout 300s -v 2>&1 \| grep -c "^--- PASS: TestAR53_"` reads 1; `go test ./internal/effects -run "^TestAR20" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./internal/gate -run "^TestAR21_" -count=1 -timeout 600s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 60 turns, 150k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

Night 2026-10-02-4 (example-app, `--until 07:00`) stopped `no-progress` at 07:15:44Z after 1 h 47 min. Row l2ax's gate: `"reason": "git-state-moved", "result": "unrun"`, detail "git state: git-state-moved: config, hooks or info changed since stage". Then "stage esfix2: git-state-moved: config, hooks, info or alternates changed since the root's first stage", the same for es3b and orient.4, and the `stop` line: `"reason": "no-progress"`. It is the first `git-state-moved` in any ledger.

What moved it: at 00:05:26–30 local, `git gc --auto`, set off by the row's commits in its worktree (the object store is shared), expired the reflogs, wrote `objects/info/commit-graph`, repacked, and ran `update-server-info`, which wrote `objects/info/packs` and created `info/refs` (it lists `refs/heads/l2ax` at a mid-sitting sha). `GitStateDigest` (`internal/effects/git.go:686-718`) hashes the common dir's `info/` as a whole (`:699`, walked by `digestTree`, `:821-856`, mode, length and bytes), so a new `info/refs` is a moved state. No other hashed path changed after the stage.

The guard exists to stop a row that plants a hook or changes config, attributes or excludes to fool the gate. `info/refs` is a listing git writes for dumb-HTTP serving; no local git command reads it.

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar53.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. `DECISIONS.md` stays as it is. Each act's test is red on `main`; put the red line in the commit message.

1. **`info/refs` is not git state (act 1).** A regular file named `refs` directly inside the common dir's `info/` is left out of the digest. Everything else stays in: `info/attributes`, `info/exclude`, `info/sparse-checkout`, `info/grafts`, any other file under `info/`, and a `refs` under `info/` that is a directory or a symlink. Tests in `internal/effects/ar53_test.go`: `TestAR53_1` (writing or rewriting `info/refs` after a digest leaves it equal); `TestAR53_2` (each of `info/exclude`, `info/attributes`, a new `info/x`, a directory `info/refs/` holding a file, and a symlink `info/refs` still changes it); `TestAR53_3` (in a temp repo with the real `git`: digest, commit, `git update-server-info`, digest again: equal; skip when `git` is missing).
2. **The detail names the entry (act 2).** When the git state moved, the gate's and the stage's detail names the first entry that differs, in this order: `config`, `config.worktree`, `hooks`, `info`, `objects/info/alternates`, the tree's `config.worktree`; for example "git-state-moved: info changed since stage". Keep the combined digest as the comparison and as the ledger's `state_digest` value; keep the per-entry digests beside it in memory, in an unexported field. No ledger key, stop reason or exported signature changes. `internal/gate/ar53_test.go`: `TestAR53_4` stages a row, changes `hooks/` in the common dir, gates, and reads `hooks` in the detail.
3. **The records (act 3).** `design/relay-r1.md`'s sentence on the git state gains "`info/refs`, which `git update-server-info` writes, is not part of it". `NICE_TO_HAVE.md`'s item leaves, with "(landed as AR53)".
4. **What this packet does not do.** It does not pass `gc.auto=0` to children or to the relay's own git calls, does not stop git's auto-gc, and does not hash or unhash anything outside `info/`. If you find another file git writes by itself into a hashed path, say so in the report and leave it.
5. **The temp file too (act 4, the review's fix round).** `git update-server-info` writes `info/refs_` plus six characters and renames it onto `info/refs`, so a detached `gc --auto` can leave one there while a digest is taken (review F1, 2026-10-03). A regular file whose name is `refs_` followed by exactly six characters, directly inside `info/`, is left out too; a directory or symlink of that name, `refs_` with any other length, and every other name stay in. `TestAR53_5`: a regular `info/refs_Ab3dEf` added after a digest leaves it equal; a directory `info/refs_Ab3dEf/`, a symlink of that name, and a regular `info/refs_ab` each change it. Add the review's three other findings to `NICE_TO_HAVE.md` as given in the hand-off.

## 3. Interfaces (signatures, not prose)

Unchanged: `GitStateDigest`'s signature and its result for every tree without an `info/refs` file; the ledger's keys; `relay.GitStateMoved`. New: unexported helpers in `internal/effects` and one unexported field where `internal/night/stage.go` keeps the binding.

## 4. Findings that name this row

`internal/effects/git.go:686-718` (`GitStateDigest`), `:699` (the paths), `:709` (the tree's `config.worktree`), `:821-856` (`digestTree`), `:600` (`InstallPrePush` rewrites `.git/config` with the same bytes at every stage); `internal/night/stage.go:389`, `:416`, `:419` (`Staged.StateDigest`), `:434-476` (`stateBase`, bound at the root's first stage), `:542-545`, `:578`; `internal/gate/gate.go:228`, `:234-245`, `:847`, `:1267`, `:1354-1367`; `internal/relay/types.go:135-137`; `internal/effects/ar20_test.go:51` (the one `info/` test).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line with its count, `go vet`'s exit, `make doclint`'s exit and `make hygiene`'s last line. Bullets: each act's red line on `main`; the detail's words; any other self-written git file found; what was left. Last line: branch, head sha.
