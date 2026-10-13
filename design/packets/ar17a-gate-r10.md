# Packet AR17A: the gate binds main, judges the branch's own tree under main's harness (ADV10-1, -2, -7, -10)

| item | value |
|---|---|
| from | `design/review-adv10.md` whole; `design/relay-r1.md` §6, §11, §16; `design/tla/`; the code the review cites. Never a ledger but a fixture's |
| pin | agent-relay `main` after `packets-34-relay` (cut off `main`, no `base`); worktree `.claude/worktrees/ar17a-gate-r10` |
| wall | this repo only. No push, `--amend`, rebase, device or network; every test path under `t.TempDir()`, never the real queue, ledger or state root; a hand `relay plan` only with `--rows`, `RELAY_MEASURE_DEPTH=1` and a temp `XDG_STATE_HOME`; `go test ./...` only inside `make check` |
| may touch | `internal/gate/`, `internal/night/`, `internal/effects/`, `internal/relay/`, `cmd/relay/`, `design/relay-r1.md`, `design/tla/Night.tla`, `design/tla/Night.cfg`, `history/sittings/*.md`, `history/sessions/NEXT` (removed if present). Nothing else |
| acceptance | `for p in gate night effects relay; do go test ./internal/$p -run TestAR17A_ -count=1 >/dev/null 2>&1 \|\| exit 1; done; echo ok` reads `ok`; `go test ./cmd/relay -run TestAR17A_ -count=1 2>&1 \| cut -c1-2 \| tail -1` reads `ok`; `grep -rhc '^func TestAR17A_' internal cmd \| paste -sd+ - \| bc` reads 7 or more; `grep -c 'TestAR17A_' design/relay-r1.md` reads 5 or more; `grep -c NoMergeAfterMainMoved design/tla/Night.cfg` reads 1; `test -e history/sessions/NEXT; echo $?` reads 1 |
| budget | 80 turns, 130k context, `claude-opus-5-5`; one commit per act (0 to 3) plus the charter's; `make check` is the verify line, never acceptance; checkpoint at 80%; the charter is the first free `history/sittings/2026-09-23-<n>-ar17a.md`, closed before the last commit |
| verify | `make check` |

## 1. What the source says (quoted)

review-adv10: "A child alone reaches three new wrong greens (1, 2, 7)"; "Fix before example-app: 1 to 7, 9, 10". The first of its three tests: "`TestADV10_GateBindsBase`: a child moves `main`, then tags `main`; each gate is red with a named reason and nothing merges."

## 2. Rulings

Decide inside the design, record each choice in the charter, at most ONE fork. One commit per act; each act's test reproduces the finding FIRST (red on `main`), then the fix. §16's rule: act 0 changes `Night.tla`; `make tla` reads `tla: ok` once before the closing commit (reported).

0. **The stage snapshots main and origin; the gate binds them (ADV10-1, -2; the ls-remote half of -3).** `Stage` records `Staged.BaseSha` (`rev-parse --verify refs/heads/<base>^{commit}`, never the bare name) and `Staged.OriginRefs` (`git ls-remote <remote>` per remote, a failure recorded `unknown`); both go on the `stage` ledger line and `StagedFromLedger` reads them back. The gate: main's sha not `BaseSha` is `red`, reason `main-moved`, and nothing merges; `DiffNames` and every range diff read `BaseSha...g.Head`, never a ref name; any git call whose stderr is non-empty or whose exit is non-zero makes its check `unrun` (`effects/git.go:23` stops dropping stderr); a fresh `ls-remote` differing from `OriginRefs` is `origin-moved` (an alarm, as §1). `TestAR17A_0_GateBindsBase` (the review's test 1: `update-ref` on main, then `git tag main HEAD`; each red with its reason, no merge), `TestAR17A_0_DiffStderrUnrun`, `TestAR17A_0_UrlPushSeen`. Model: `mainSha` recorded at stage; `NoMergeAfterMainMoved` in `Night.cfg`.
1. **The gate judges the commit, not the tree (ADV10-7).** Acceptance and verify lines run in a fresh detached worktree at `g.Head` (as `Hand` does), created under the state root and removed after; the child's tree is read only for `clean`. A `skip-worktree`, `assume-unchanged` or `info/exclude` trick changes nothing the gate runs. `TestAR17A_1_SkipWorktreeCannotFake`, `TestAR17A_1_ExcludeCannotHide`.
2. **Main's harness (ADV10-10).** The gate's and the merge's verify line run with main's `Makefile` and `scripts/` at `BaseSha` laid over the fresh tree (`git checkout <BaseSha> -- Makefile scripts` inside it); `GuardedPaths` gains `Makefile` and `scripts/` (named exactly in may-touch, as today). A branch that changes the harness is judged under main's, and its merge line notes `harness changed`. `TestAR17A_2_BranchCannotNeuterVerify`.
3. **The design.** relay-r1 §6 gains the base binding, the fresh tree and main's harness; §11's table names every `TestAR17A_`; §16 maps `NoMergeAfterMainMoved` to `TestAR17A_0_GateBindsBase`.

## 3. Interfaces (signatures, not prose)

- `night.Staged.BaseSha string`, `.OriginRefs map[string]string`; `effects.Git.RevParseCommit(dir, ref string) (string, error)`, `.LsRemote(dir, remote string) (map[string]string, error)`; `gate.Fresh(fx, root, sha string) (dir string, done func(), err error)`; the reason strings `main-moved`, `harness changed`.

## 4. Findings that name this row

Baseline on `main` after packets-34-relay: ADV10-1 reproduces with `git update-ref refs/heads/main HEAD` in a linked worktree; ADV10-2 at `gate.go:123`; ADV10-7 at `gate.go:108`, `:141`, `:261`; ADV10-10 at `gate.go:39`.

## 5. Forbidden reads

Every other repo; `~/.claude/projects/`; the real state root; any ledger but a fixture's; the packets before `ar13-*`; `.claude/worktrees/` but your own.

## 6. Report (capped)

The table: each acceptance line, its exit and last line. Bullets: per act the reproducing test's red line on `main`; `make tla`'s last line; what was left undone; the one open fork. Last line: the branch and its head sha.
