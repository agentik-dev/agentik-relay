# Packet AR50: the pin's asks are bounded, and AR49's tests hold what they say

| item | value |
|---|---|
| from | `NICE_TO_HAVE.md`, "AR49's review": its first four items, each with a refuter's reproduction; `design/pin-pycache-r1.md` §2 |
| pin | agent-relay `main` at 0567593 plus this packet's own commit; worktree `.claude/worktrees/ar50-pin-ask-bounded`, branch `ar50-pin-ask-bounded` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state: a real night runs until 07:00. No `make check` and no `go test ./...`: `go test ./internal/effects` and `make doclint` only. **Python:** every shell line you run carries `PYTHONDONTWRITEBYTECODE=1` (`make doclint` among them), and a python you start is `python3 -B`. A test that runs the real python imports only a module the test wrote in its own temp dir, never a stdlib module by name |
| may touch | `internal/effects/tree.go`, `internal/effects/ar50_test.go`, `internal/effects/ar49_test.go` (ruling 2 only), `design/relay-r1.md`, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/effects -run "^TestAR(49\|50)_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR[45][09]_"` reads 8 |
| budget | 40 turns, 120k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

`NICE_TO_HAVE.md`, "AR49's review": "The ask at the pin can outlast its 30 s: `pycacheHonored` (`internal/effects/tree.go:76-83`) reads with `cmd.Output()` and sets no `WaitDelay`, so a python that leaves a child holding stdout holds the pin until that child exits (50 s with a fake whose ask runs `sleep 50 & wait`; a holder that never exits hangs the pin). `toolRoots` has the same shape."

"`TestAR49_1_ScratchPrefix`'s check that the scratch's bytecode dir is gone after the remove func proves nothing: `pyc` is `Run`'s last line, cut at `LastCap` (200 bytes), so the path it stats never existed. With a 141-byte `TMPDIR` the test fails".

"The predicate's root clauses (`tree.go:138`, `:147`) have no test: with both deleted all five `TestAR49_` tests pass". "The predicate's ASCII-only digits (`tree.go:170`) have no test: with `unicode.IsDigit` swapped in, all five pass."

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar50.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. `DECISIONS.md` stays as it is. No behavior of the predicate changes: acts 2 and 3 change tests only. No existing test line changes outside ruling 2.

1. **Both asks end on time** (act 1). `pycacheHonored` and `toolRoots` each set `cmd.WaitDelay`, so a child that holds stdout cannot hold the pin. The timeout (30 s) and the wait become two unexported package variables that a test shortens and restores; their values on a real pin stay 30 s and a wait you choose, at most 5 s. When the wait closes the pipes, the ask is not honored and the pin succeeds, as for any error; a `toolRoots` call that ends that way is an error and the pin fails `unpinned-tool`, with its roots unread. Test `TestAR50_1_AsksBounded`, with `fakeTool` and both variables set to 1 s: a fake whose prefix ask runs `sleep 20 & wait` gives a pin with no error and a note, in under 10 s; a fake whose prefix ask starts `sleep 20 &`, answers and exits 0 gives the same; a fake whose roots answer does the same gives an error that `errors.Is` reads as `ErrUnpinnedTool`, in under 10 s; a fake that `exec`s `sleep 300` on the ask gives a pin with no error and a note, in under 10 s. Each case kills what it left running at cleanup. Red on `main`: the first case takes 20 s.

2. **`TestAR49_1_ScratchPrefix` checks the file it means** (act 2). The scratched part of that test (`ar49_test.go:76-96`) moves into one helper. Its second line prints the found file's path relative to `$PYTHONPYCACHEPREFIX`, so `Run`'s last line is short whatever the temp dir. The helper joins it to the scratch's own `pycache` dir (`sh.(OSShell).Scratch`), reads the file there before the remove func, and reads both the file and the dir as gone after it. `TestAR49_1_ScratchPrefix` calls the helper and changes in no other line. New test `TestAR50_2_LongTmpdir` sets `TMPDIR` to a dir of its own at least 141 bytes long before it makes any temp dir, pins the real python3 as `TestAR49_1_ScratchPrefix` does (skipped when the pin reaches none), and calls the helper. Red on `main`: the helper's body as it stands fails there under that `TMPDIR`; show it in the report.

3. **The predicate's two untested edges** (act 3). Test `TestAR50_3_PredicateEdges`, under AR49's honoring fake. A root that is itself named `__pycache__` (the fake names a dir of that name as its only root): a tagged `.pyc` added directly in it after the pin is moved by `Moved()` and, in a second fresh pin, by `MovedStrict()`. In a real `__pycache__` below an ordinary root: `x.cpython-３１２.pyc` (fullwidth digits) and a name with Arabic-Indic digits in the tag are each moved, and so is `x.cpython-312.opt-２.pyc`. These pass on `main`. Each must be red under its mutant, shown in the report and then restored: the root clauses (`path == s.real`, `dir != s.real`) deleted; `unicode.IsDigit` in `digits`.

After the acts: `NICE_TO_HAVE.md` loses the four items this packet lands, in "AR49's review", and keeps the rest; `design/relay-r1.md` names the new tests where `make doclint` asks for them (§11).

## 3. Interfaces (signatures, not prose)

New: two unexported package variables in `internal/effects/tree.go` (the asks' timeout and their wait); one unexported test helper in `ar49_test.go`. Unchanged: the signatures of `pycacheHonored`, `toolRoots`, `PinAt`, `Moved`, `MovedStrict` and `PycacheNote`; `pyOwn`, `cacheTagged` and `digits` byte for byte; every ledger key and stop reason.

## 4. Findings that name this row

`internal/effects/tree.go:64-84` (`pyPrefix`, `pycacheHonored`), `:86-121` (`toolRoots`), `:126-175` (`pyOwn`, `cacheTagged`, `digits`); `internal/effects/ar49_test.go:20-105` (`TestAR49_1_ScratchPrefix`), `:107-` (the honoring fake); `internal/effects/ar24_0_test.go:13-40` (`fakeTool`, `tree`); `internal/effects/pin.go` (`ErrUnpinnedTool`, the roots and the ask in `PinAt`); `internal/effects/shell.go` (`LastCap`, `OSShell.Scratch`).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: the acceptance line's exit and last line, the last line of the whole `go test ./internal/effects -count=1`, and `make doclint`'s. Bullets: act 1's red run on `main` and the wait you chose; act 2's red run under the long `TMPDIR`; act 3's two mutants, each test's result under it, and that both are restored; what was left. Last line: branch, head sha.
