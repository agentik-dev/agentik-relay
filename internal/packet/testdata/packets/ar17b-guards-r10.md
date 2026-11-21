# Packet AR17B: the child's guards close by shape, fail closed, and keep the repo's hooks (ADV10-3, -6, -11, -15, -18, -20)

| item | value |
|---|---|
| from | `design/review-adv10.md` whole; `design/relay-r1.md` §7, §10, §11, §15; the code the review cites. Never a ledger but a fixture's |
| pin | agent-relay `main` after `ar17a-gate-r10` merged (cut off `main`, no `base`); worktree `.claude/worktrees/ar17b-guards-r10` |
| wall | this repo only. No push, `--amend`, rebase, device or network; every test path under `t.TempDir()`, never the real queue, ledger or state root; a hand `relay plan` only with `--rows`, `RELAY_MEASURE_DEPTH=1` and a temp `XDG_STATE_HOME`; `go test ./...` only inside `make check` |
| may touch | `internal/hooks/`, `internal/effects/`, `internal/night/`, `cmd/relay/`, `plugin/hooks/run`, `plugin/hooks/hooks.json`, `design/relay-r1.md`, `history/sittings/*.md`, `history/sessions/NEXT` (removed if present). Nothing else |
| acceptance | `for p in hooks effects night; do go test ./internal/$p -run TestAR17B_ -count=1 >/dev/null 2>&1 \|\| exit 1; done; echo ok` reads `ok`; `go test ./cmd/relay -run TestAR17B_ -count=1 2>&1 \| cut -c1-2 \| tail -1` reads `ok`; `grep -rhc '^func TestAR17B_' internal cmd \| paste -sd+ - \| bc` reads 8 or more; `grep -c 'TestAR17B_' design/relay-r1.md` reads 6 or more; `test -e history/sessions/NEXT; echo $?` reads 1 |
| budget | 80 turns, 130k context, `claude-opus-5-5`; one commit per act (0 to 4) plus the charter's; `make check` is the verify line, never acceptance; checkpoint at 80%; the charter is the first free `history/sittings/2026-09-23-<n>-ar17b.md`, closed before the last commit |
| verify | `make check` |

## 1. What the source says (quoted)

review-adv10: "A child ... can push (3)"; "three doors remain: ... `go run` (6)"; "11 matters: example-app has commit hooks". AR17A already compares `ls-remote` against the stage snapshot, so a push that lands is seen at the gate; this packet stops it at the door.

## 2. Rulings

Decide inside the design, record each choice in the charter, at most ONE fork. One commit per act; each act's test reproduces the finding FIRST (red on `main`: the hook returns `{}` for the review's spelling), then the fix. The guards close by shape, never by a list of spellings: a segment whose command word is not a literal word is denied under `RELAY_ROW`.

0. **Push by any door (ADV10-3).** Under `RELAY_ROW` the hook denies: `git send-pack`, `git http-push`, `git subtree push`, `git push` in every wrapper `hooks.Segments` unwraps (`env -S`, `xargs`, `bash <<<`, `sh -c`); a command word that starts with `$` or `$(` or a backtick; a shell (`sh`, `bash`, `zsh`, `dash`) fed from stdin, a here-string or a pipe. `TestAR17B_0_PushDoorsDenied` holds every spelling the review landed; `TestAR17B_0_LiteralWordsStillAllowed` keeps `git status`, `go test ./internal/x -run T` and `make fmt` allowed.
1. **Main and the hooks stay the root's (ADV10-1's door, -11).** Denied under `RELAY_ROW`: `git update-ref`, `git branch -f|-M|-D` naming the base, `git -C <root>` and `--git-dir`/`--work-tree` naming the root, any `config` write to `core.hooksPath`. `effects/git.go:261`'s hooks directory links every hook of the root's common hooks directory beside its `pre-push`, so a `pre-commit` that refused before still refuses. `TestAR17B_1_UpdateRefDenied`, `TestAR17B_1_RepoCommitHooksRun`.
2. **Nested runs by any spelling (ADV10-6, -18).** `NestedRun` matches a segment whose argv holds a `cmd/relay` path or a binary whose base name is `relay`, with `run`, `plan`, `gate` or `resume` anywhere after it; `env -u` changes nothing. The binary: `run`, `plan` and `resume` write `<state>/relay.pid` (`pid started`) and refuse, exit 2 with `nested: relay <pid> is live`, while a live, start-time-matching holder that is an ancestor of this process exists; a dead or reused pid is taken over. `relay resume` refuses under `Nested` like `run`. `TestAR17B_2_GoRunRelayDenied`, `TestAR17B_2_AncestorPidRefuses`, `TestAR17B_2_ResumeNested`. §16's `NoNestedSpawn` maps to them.
3. **Fail closed (ADV10-15).** `ChildEnv` sets `RELAY_BIN` to the running binary's absolute path (`os.Executable`); `plugin/hooks/run` runs `$RELAY_BIN` when set, else `relay` on PATH, and when neither exists prints a deny under `RELAY_ROW` (exit 0 with the deny JSON) and `{}` outside a row. `TestAR17B_3_NoBinaryDeniesInRow`.
4. **The lows that share the hook (ADV10-20) and the design.** The amend and rebase guards add `reset --soft`, `commit-tree` and `-c pull.rebase=*`/`pull --rebase`: `TestAR17B_4_HistoryRewritesDenied`. relay-r1 §7 and §10 state the by-shape rule and list the denied verbs once; §11's table names every `TestAR17B_`.

## 3. Interfaces (signatures, not prose)

- `hooks.Verdict(seg string, inRow bool) (deny bool, why string)` stays the one entry; `hooks.CommandWordLiteral(seg string) bool`; `effects.ChildEnv(environ []string, row string, remotes []string, bin string)`; `night.PidFile(state string) (release func(), err error)`; the env var `RELAY_BIN`.

## 4. Findings that name this row

Baseline on `main` after ar17a merged: ADV10-3 at `hooks/guards.go:28`; ADV10-6 at `guards.go:42`; ADV10-11 at `effects/git.go:261`; ADV10-15 at `plugin/hooks/run:4`; ADV10-18 at `main.go:1087`; ADV10-20 by the review's three spellings.

## 5. Forbidden reads

Every other repo; `~/.claude/projects/`; the real state root; any ledger but a fixture's; the packets before `ar13-*`; `.claude/worktrees/` but your own.

## 6. Report (capped)

The table: each acceptance line, its exit and last line. Bullets: per act the reproducing test's red line on `main`; what was left undone; the one open fork. Last line: the branch and its head sha.
