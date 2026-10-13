# Packet AR49: python's own bytecode leaves the pinned tree alone

| item | value |
|---|---|
| from | `design/pin-pycache-r1.md` §2 (the decision), §3 (the seam) and §5 item 1; read its §7 first |
| pin | agent-relay `main` at cc7ab7b plus this packet's own commit; worktree `.claude/worktrees/ar49-pin-pycache`, branch `ar49-pin-pycache` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state. No `make check` and no `go test ./...` (a relay night is live): `go test ./internal/effects`, `go test ./cmd/relay -run Doctor` and `make doclint` only. **Python:** every shell line you run carries `PYTHONDONTWRITEBYTECODE=1` (`make doclint` among them), and a python you start is `python3 -B`. A test that runs the real python imports only a module the test wrote in its own temp dir, never a stdlib module by name: a first import of one turns the live night's gate unrun |
| may touch | `internal/effects/scratch.go`, `pin.go` and `tree.go`, `internal/effects/ar49_test.go`, `internal/effects/ar22_1_test.go` (the `pinnedKeys` line only), `cmd/relay/main.go` (one doctor row, ruling 2), `design/relay-r1.md`, `design/pin-pycache-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/effects -run "^TestAR49_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR49_"` reads 5 |
| budget | 60 turns, 150k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

`design/pin-pycache-r1.md` §2: "The gate's python stops reading and writing the shared `__pycache__`, and then the snapshot leaves python's own cache files out. It leaves out those files only, never the directory's other contents."

"Under a python3 root, two things are not entries: a real directory named `__pycache__` (so one that python makes after the pin is not an entry added), and a regular file directly inside one whose name is `<name>.cpython-<digits>.pyc` or `<name>.cpython-<digits>.opt-<n>.pyc`. Everything else inside a `__pycache__` is an entry as anywhere".

What was seen (§1): on 2026-10-01 a python outside the night wrote two `.pyc` files into the stdlib, and the night's gate was `unrun`, `tool-moved: python3`.

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar49.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's test is red on `main`. `DECISIONS.md` stays as it is. **One existing test line changes:** `pinnedKeys` (`internal/effects/ar22_1_test.go:20-24`) gains `PYTHONPYCACHEPREFIX`. If another existing test asserts the exact set of scratch dirs or env names, add the name there and say so in the report; change no other existing test line.

1. **Each scratch has its own bytecode dir, and the pin's asks write none** (act 1). `scratchDirs` gains `pycache`. `scratchEnv` sets `PYTHONPYCACHEPREFIX=<scratch home>/pycache` and drops the one it was handed. `Pinned.Env` gains `PYTHONPYCACHEPREFIX=/dev/null/pycache`, set before `toolRoots` runs; it never gains `PYTHONDONTWRITEBYTECODE`. Test `TestAR49_1_ScratchPrefix`, in `TestAR25_1_UserSiteOff`'s shape (the real python3): the pinned env holds that entry. A fake root is a temp dir that holds `victim.py`. An `OSShell{}` line whose python puts the root on `sys.path` and imports `victim` leaves no `__pycache__` in the root. Under `Scratched`, a first line does the same import, and a second line finds `victim.cpython-*.pyc` under its `$PYTHONPYCACHEPREFIX`, in the scratch home, gone after its remove func. A python run as `toolRoots` runs one (the `bin/` link, `p.Env`) that imports `victim` writes no file under the root.

2. **The pin asks its python once** (act 2). After `toolRoots` and before `snapRoots`, the pin asks the pinned python, under `p.Env`, for `sys.pycache_prefix`. The prefix is honored only when the answer is exactly `/dev/null/pycache`; any other answer, an error or a timeout is not honored, and the pin still succeeds. `TestAR24_0_PythonRoots` stands unchanged: its fake records its last call as the pinned `HOME` and `-c`, and answers every call with its roots, which must read as not honored. How the ask is made is yours. New `PycacheNote(p Pinned) string`: empty when the prefix is honored or no python3 is pinned, else one line that names python3 and `__pycache__` and says the cache files stay counted. `relay doctor` adds one warning row from it beside the breaker's (`cmd/relay/main.go:262`), and no row when it is empty. The design says "the plan"; doctor tells the pin's other facts, so the line is doctor's. Test `TestAR49_2_AskAtPin`, with `fakeTool`: a fake that answers the ask with its `$PYTHONPYCACHEPREFIX` gives an empty note; AR24's fake gives a note; a fake that exits 1 on the ask gives a note and no pin error; a pin with no python3 gives an empty note.

3. **Python's own cache files are not entries** (act 3). One predicate in `tree.go`, used by `snapRoots`, `moved` and `movedStrict`, so the three agree. It holds only for a snapshot whose tool is `python3` alone (one shared with `go` is walked whole) and only when the prefix is honored. It leaves out a real directory named `__pycache__` whose parent is not named `__pycache__`, and a regular file directly inside such a directory named `<name>.cpython-<digits>.pyc` or `<name>.cpython-<digits>.opt-<digits>.pyc`, no part empty. The walk still descends: everything else in the directory is an entry. The walk adds no stat. Test `TestAR49_3_CacheFilesNotMoved`, with a fake that honors the prefix and a root that holds `lib/python3.12/os.py` and `lib/python3.12/__pycache__/os.cpython-312.pyc`: after the pin, add `pty.cpython-312.pyc` and `pty.cpython-312.opt-2.pyc` there and change the bytes of `os.cpython-312.pyc`; in a second case remove it. `Moved()` and then `MovedStrict()` name nothing.

4. **Everything else is still moved** (act 4). Test `TestAR49_4_StillMoved`, a table under the same fake: each case is added after a fresh pin and is named `python3` by `Moved()` and, in a second fresh pin, by `MovedStrict()`. The cases: a `.pth`; a `sitecustomize.py`; `x.cpython-312.pyc` outside any `__pycache__`; a regular file named `__pycache__`; a symlink named `__pycache__` to a directory; and inside the real `__pycache__`: `evil.py`, `evil.so`, `evil.pyc`, `.cpython-312.pyc`, `x.cpython-.pyc`, `x.cpython-312.opt-.pyc`, a subdirectory, a directory named `x.cpython-312.pyc`, a symlink named `x.cpython-312.pyc`, and a tagged `.pyc` in a `__pycache__` nested in it. One more case pins a fake `go` whose root holds a `__pycache__`: a tagged `.pyc` added there is moved, naming `go`.

5. **A new cache directory, and a python that does not honor the prefix** (act 5). Test `TestAR49_5_NewDirAndUnhonored`. Under the honoring fake: pin, `Moved()`, then make a `__pycache__` with a tagged `.pyc` in a directory that had none. `MovedStrict()` names `python3`, since the parent's stat changed after the gate read it (§6; it fails closed). Then `Moved()` names nothing, and so does a `MovedStrict()` after it. Under AR24's fake a tagged `.pyc` added to an existing `__pycache__` is moved by `Moved()`.

After the acts: `design/relay-r1.md` §6 gains AR49's sentences in *The toolchains' trees* and in **The relay home bound**, each naming its test; follow what `make doclint` asks of §11. `design/pin-pycache-r1.md`'s status line says AR49 built §5 item 1, and its §3 says the line is doctor's.

## 3. Interfaces (signatures, not prose)

New: `func PycacheNote(p Pinned) string` in `internal/effects`; one unexported field on `Pinned`; one unexported predicate in `tree.go`; the env name `PYTHONPYCACHEPREFIX` in `Pinned.Env` and `scratchEnv`; the scratch dir `pycache`. Unchanged: the signatures of `Moved`, `MovedStrict`, `PinAt`, `Scratched` and `toolRoots`; every ledger key and stop reason; `Pinned`'s exported fields.

## 4. Findings that name this row

`internal/effects/scratch.go:44`, `:129-151` (`scratchEnv`); `pin.go:87-99` (`Pinned`), `:275-287` (the pinned env), `:298-320` (the roots, the snapshot); `tree.go:60-97` (`pyRoots`, `toolRoots`), `:103-158` (`snapRoots`), `:200-231` (`moved`), `:248-284` (`movedStrict`); `shell.go:121` (a line's env); `ar24_0_test.go:13-90` (`fakeTool`, `tree`); `ar25_1_test.go:14-41`; `procs.go:162-173` (`ExeReader`); `cmd/relay/main.go:259-267`; `design/relay-r1.md:139`, `:146`.

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: the acceptance line's exit and last line, the last line of the whole `go test ./internal/effects -count=1`, and `make doclint`'s. Bullets: each act's red test on `main`; how the ask is made; the note's words; each existing test line changed; what was left. Last line: branch, head sha.
