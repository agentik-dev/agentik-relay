# agent-relay design: python's own bytecode and the pinned tree (r1)

**Status:** design r1, 2026-10-01, written before code and revised once after its one refuter round (§7), which blocked on one finding, now taken. It amends relay-r1 §6 (the gate): *The toolchains' trees* and **The relay home bound**. No entry of `DECISIONS.md` covers the pin's tree. Proof P1 ran (§4). AR49 built §5 item 1 (2026-10-01): relay-r1 §6 holds the rule as built, and its §11 names the five `TestAR49_` tests.

## 1. The problem

On 2026-10-01 a night's gate was `unrun`, `tool-moved: python3`, and its row ended `red: gate-unrun` with five acts unjudged. No tool had moved. The relay pinned its tools at 14:50:57. At 14:58:09 a process outside the night ran the same Homebrew python and imported `pty`, for the first time on this Mac under that python, and python wrote `pty.cpython-314.pyc` and `tty.cpython-314.pyc` into the stdlib's `__pycache__`. Those are two entries added to python3's tree, and `treeSnap.moved` (`internal/effects/tree.go:200`) counts an added entry as the tool moved. The writer's timestamp is the two files' mtime to the second, and nothing else in python's roots changed in the pin's window. It is the first `tool-moved` in any ledger.

Any python on the account can do this at any time: a hook, a plugin script, a `make doclint`, a row's own `python3 - <<EOF`. The row's sitting runs under the account's env, so a row can turn its own gate unrun by importing a stdlib module nobody imported before.

The entry cannot simply be ignored. Python loads `__pycache__/X.cpython-NN.pyc` in place of `X.py` when the file's header carries the source's mtime and size, and it does not look at the code inside. A `.pyc` planted there changes what the stdlib does for every python that reads it (§4, arm 1). That is why AR24 counted the whole tree.

## 2. The decision

The gate's python stops reading and writing the shared `__pycache__`, and then the snapshot leaves python's own cache files out. It leaves out those files only, never the directory's other contents.

- **Each scratch names its own bytecode dir.** `scratchEnv` sets `PYTHONPYCACHEPREFIX=<scratch home>/pycache`, a dir the scratch makes fresh and removes with itself, as it does `GOCACHE`. With the prefix set, python reads and writes bytecode only in a mirror tree under it and never beside the source (§4, arms 2 and 3). So a gate's, a merge verify's and a baseline's lines compile what they import once, into a dir nothing else has written, and leave nothing in the tool's tree.
- **The runner's own asks write nothing.** The pinned env (`Pinned.Env`, which `toolRoots` runs python under) sets `PYTHONPYCACHEPREFIX=/dev/null/pycache`, a path no process can make, so those asks read no bytecode and write none. It does not set `PYTHONDONTWRITEBYTECODE`: a line's env is built from the pin's (`shell.go:121`), and that name would stop the scratch's prefix from being written. `scratchEnv` replaces the prefix with the scratch's own.
- **The snapshot leaves python's cache files out**, at the pin and at each `moved` and `movedStrict`. Under a python3 root, two things are not entries: a real directory named `__pycache__` (so one that python makes after the pin is not an entry added), and a regular file directly inside one whose name is `<name>.cpython-<digits>.pyc` or `<name>.cpython-<digits>.opt-<n>.pyc`. Everything else inside a `__pycache__` is an entry as anywhere: a `.py`, a `.so`, a `.pyc` of any other name, a subdirectory, a symlink. A `__pycache__` is an importable namespace package, so a source or an extension planted in one runs under the prefix (§7, F1); a cache-tagged `.pyc` there cannot be imported by name. A file or a symlink named `__pycache__` is an entry. Go's tree is walked whole, as today.
- **Only when the pinned python honors the prefix.** At the pin the relay asks the pinned python, under an env with the prefix set, for `sys.pycache_prefix`. If the answer is not that path (a python before 3.8, or one that drops the variable), the snapshot keeps `__pycache__` in, as today, and the plan says so in one line.

What stays counted: every `.py`, `.so`, `.pth` and `sitecustomize.py`, inside a `__pycache__` or not; a `.pyc` whose name carries no cache tag, wherever it lies (python imports one where there is no `.py`); every other entry added, removed or changed in a root.

## 3. The seam and the record

- `internal/effects/scratch.go`: `scratchDirs` gains `pycache`; `scratchEnv` names it. `internal/effects/pin.go`: the two names in `Pinned.Env`, and the ask at the pin. `internal/effects/tree.go`: one predicate for "python's own cache entry" (the directory, or a cache-tagged regular file directly in it, below a python3 root), used by the snapshot and by both `moved` walks, so the three agree.
- `Pinned` gains one unexported field: whether the snapshot left `__pycache__` out. When it did not, one line says so, and the line is doctor's, not the plan's (AR49, the packet's ruling 2): `relay doctor` tells the pin's other facts, and adds a warning row, `python3`, from `effects.PycacheNote`.
- No ledger key changes. `tool-moved` keeps its meaning for every other entry.
- The existing tests of the tree (`TestAR24_0_PythonRoots` and its neighbours) plant a `.pth` and a `sitecustomize.py`; they stand unchanged, and no test plants in a `__pycache__`. One existing test line changes: `pinnedKeys` (`internal/effects/ar22_1_test.go:20-24`) gains the new name.

## 4. The proof

**P1, ran 2026-10-01 16:05, Homebrew python 3.14.7, in a scratch dir, every python `-B` except arm 3.** A module `victim.py` holds `WHO = "source"`. A `.pyc` compiled from `WHO = "planted pyc"`, with a header carrying `victim.py`'s own mtime and size, is placed in `__pycache__` beside it.

| arm | env | `WHO` | bytecode read from |
|---|---|---|---|
| 1 | no prefix | `planted pyc` | `mods/__pycache__/` |
| 2 | `PYTHONPYCACHEPREFIX` set, `-B` | `source` | the prefix (empty) |
| 3 | `PYTHONPYCACHEPREFIX` set, writes on | `source` | the prefix; the new `.pyc` is written under it, nothing beside the source |
| 4 | prefix set, the planted `.pyc` copied into the prefix | `planted pyc` | the prefix |

Arm 1 is the attack, so today's rule is not idle. Arms 2 and 3 are the decision: with the prefix set, what lies in `__pycache__` beside a source does not reach the import. Arm 4 is the limit: the prefix dir is as trusted as the scratch home it lies in, which is why it is the scratch's and fresh, not a dir that lives the night.

## 5. The cut into packets

1. **AR49, the pin.** The scratch's `pycache`, the two names in the pinned env, the ask at the pin, the predicate, the plan's line, §6's text. Acceptance by named tests in `internal/effects`: a `.pyc` added under a python3 root's `__pycache__` after the pin is not moved, by `moved` and by `movedStrict`; a `.pth`, a `sitecustomize.py`, a `.pyc` outside `__pycache__`, a file named `__pycache__`, and a `.py`, a `.so`, an untagged `.pyc` and a subdirectory inside a `__pycache__` are each still moved; a `__pycache__` directory made after the pin is not moved by `moved`; a line's python under the scratch env writes its bytecode under the scratch and none in the tree, shown with a fake root; a pinned python that does not honor the prefix keeps `__pycache__` counted.
2. **Outside any row.** This repo's own python calls (`$(PY) scripts/…` in the Makefile, the tests that run `scripts/bounded`, and `internal/packet/ar18_test.go:30-48`, which runs `scripts/doclint`) take `-B`, so a check or a test here never adds to a pinned tree while an older relay runs. **Landed as AR52** (2026-10-02, packet `ar52-own-python-b`): the Makefile's four `$(PY)` lines and the test lines that start python by `py` take `-B`, held by `TestAR52_1` and `TestAR52_2` in `internal/packet/ar52_test.go`.

## 6. Open risks

- **A python that does not carry the variable.** The prefix is an env name, so any python a line starts without it reads cache files beside the source again, and those files are no longer counted: `python3 -E` or `-I`, `env -i`, `env -u`, an empty value, a child started with a cleaned env, `sys.pycache_prefix = None` before an import, or `-X pycache_prefix` pointed into a `__pycache__` (each shown in §7, F2). A row can write such a line into its own Makefile. It gains nothing it could not get by editing that Makefile's check directly (the gate runs the row's own tree, `gate.go:398`). `-X pycache_prefix` forced on every python would close the class and needs `bin/python3` to be a wrapper, not a link; left out of r1.
- **Detection lost within a night.** Today a cache file planted in the stdlib during a night stops that night. After this the relay does not see it. A later night never saw it under either rule: the pin binds the tree as it finds it at the start.
- **The merge can still stop on a new directory.** `movedStrict` compares every entry's stat, directories included (`tree.go:273`). A first import that makes a new `__pycache__` changes its parent's ctime, so a merge whose gate ran before it is still `tool-moved`. It fails closed, and the window is the gate to its merge, not the night. 116 of 200 stdlib dirs that hold a `.py` have no `__pycache__` yet. Telling such a change from an entry added and removed needs the directory's birth time; left out of r1.
- **Cold bytecode at each gate.** Each scratch compiles what its lines import. Not measured.
- **Other self-written files.** Nothing else was seen written into a python or Go tree by the tool itself. "The first `tool-moved` in any ledger" is a count over the ledgers on this Mac. A second kind would need its own evidence.

## 7. The refuter round (2026-10-01, 16:10 to 16:19)

One refuter, read-only, every probe in a fake root. It blocked on F1; the note above is revised for all four. P1's four arms reproduced exactly. It found §1's and §3's facts about the code right, `DECISIONS.md` silent on the pin, and the Makefile claim true for a row's own gate.

1. **F1, blocking, taken.** r1 left the whole `__pycache__` directory out. A real `__pycache__` is a namespace package: with the prefix set and no `-E`, `import __pycache__.evil` ran a planted `evil.py`, as did `pkg.__pycache__.evil`, `-m __pycache__.evil`, a sourceless `evil2.pyc`, and `pkgutil.walk_packages` over a planted `__init__.py`. §2 now leaves out only the directory entry and the cache-tagged files in it.
2. **F2, taken.** `-E` and `-I` are two of a class; §6 now names the class.
3. **F3, taken as a named remainder.** `movedStrict` still stops on a new `__pycache__` directory's parent (§6).
4. **F4, taken.** `PYTHONDONTWRITEBYTECODE=1` in the pin's env would reach every line and leave the scratch's prefix empty. §2 now uses a prefix that cannot be made. A prefix under the relay home "that is never made" could be made by any process of the account.
