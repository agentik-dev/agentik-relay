# Packet AR52: this repo's own python calls write no bytecode

| item | value |
|---|---|
| from | `design/pin-pycache-r1.md` §5 item 2 ("Outside any row") and its §1 (the problem); `NICE_TO_HAVE.md`, the item "This repo's own python calls write bytecode" |
| pin | agent-relay `main` at the commit that lands this packet; worktree `.claude/worktrees/ar52-own-python-b`, branch `ar52-own-python-b` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state: a real night may be live. No `make check` and no `go test ./...`: the acceptance lines below, `go vet ./internal/packet ./internal/night`, `make -n doclint`, `make doclint` and `make hygiene` only, one at a time. **Python:** every shell line carries `PYTHONDONTWRITEBYTECODE=1`, and a python you start is `python3 -B`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `Makefile` (the four `$(PY)` recipe lines only), `internal/packet/ar52_test.go` (new), `internal/packet/ar18_test.go`, `internal/night/ar17c_test.go` and `internal/night/ar48_test.go` (the lines that start python only), `design/pin-pycache-r1.md` §5, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else; `scripts/` does not change (two of its files are pinned by sha256) |
| acceptance | `go test ./internal/packet -run "^TestAR52_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR52_"` reads 2; `go test ./internal/packet -run "^TestAR18_" -count=1 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./internal/night -run "^TestAR17C_" -count=1 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./internal/night -run "^TestAR48_" -count=1 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 40 turns, 150k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

`design/pin-pycache-r1.md` §5 item 2: "**Outside any row.** This repo's own python calls (`$(PY) scripts/…` in the Makefile, the tests that run `scripts/bounded`, and `internal/packet/ar18_test.go:30-48`, which runs `scripts/doclint`) take `-B`, so a check or a test here never adds to a pinned tree while an older relay runs."

§1: "Any python on the account can do this at any time: a hook, a plugin script, a `make doclint`, a row's own `python3 - <<EOF`."

What was seen (§1): on 2026-10-01 a python outside the night wrote two `.pyc` files into the stdlib's `__pycache__`, and the night's gate was `unrun`, `tool-moved: python3`. AR49 made the relay leave python's own cache files out of its snapshot. This packet stops this repo's own python from writing them at all, so a relay older than AR49, or a pinned python that does not honor the bytecode prefix, is not moved by a check or a test run here.

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar52.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. `DECISIONS.md` stays as it is. Each act's test is red on `main`; put the red line in the commit message.

1. **The Makefile (act 1).** Each recipe line that starts python reads `$(PY) -B scripts/…`. There are four: `check` (`scripts/bounded`), `doclint`, `hygiene` and `validate` (`scripts/bounded`). The `PY :=` line and every other word of those four lines stay as they are. `TestAR52_1_MakefilePythonTakesB` reads the repo's `Makefile` (from `../..`, as the package's other tests reach the repo), and fails naming the line number of any line that holds `$(PY)`, is not the `PY :=` assignment, and does not hold `$(PY) -B `. It also fails when it finds fewer than four such lines, so it cannot pass on an empty read. Show `make -n doclint` printing the `-B` in the report.

2. **The tests (act 2).** Every test line that starts python passes `-B` as python's first argument. Three do not today: `internal/night/ar17c_test.go:96` (`runBounded`), `internal/night/ar48_test.go:79`, and `internal/packet/ar18_test.go:48`. One shape holds for all, so a test can read it: after the interpreter variable `py` comes the literal `"-B"`, either as the next argument (`py, "-B", …`) or as the first element of the slice the line spreads (`py, append([]string{"-B", …}, …)...`). `ar48_test.go:457-459` builds `argv` with `"-B"` first and then passes `py, argv...`; rewrite it to one of the two shapes without changing what python is given. `TestAR52_2_TestsPythonTakeB` reads every `*_test.go` under the repo's `internal/` and `cmd/`, finds each line that calls `exec.Command` or `exec.CommandContext` with `py` as the program, and fails naming file and line for any that is in neither shape. It fails when it finds fewer than eight such lines. It does not match its own source. A python started under another variable name is out of scope; say in the report if you find one, and bring it into the shape if it is in a file you may touch.

3. **No python runs in the new tests.** Both `TestAR52_` tests read files and start nothing. The tests you edit (`TestAR17C_`, `TestAR18_`, `TestAR48_`) already run the real python against copies of `scripts/bounded` and `scripts/doclint`; they must pass as before, and the last three acceptance lines read that.

4. **The records (act 3).** `design/pin-pycache-r1.md` §5 item 2 gains "Landed as AR52" with the date. The `NICE_TO_HAVE.md` item "This repo's own python calls write bytecode" leaves. Nothing else in either file moves.

5. **What this packet does not do.** It does not change `scripts/bounded`, `scripts/doclint` or `scripts/hygiene`, their `#!` lines, or the two pinned sha256s. It does not add an `export` of `PYTHONDONTWRITEBYTECODE` to the Makefile. It does not touch the relay's own code under `internal/effects`, or what a row's lines run under. If you think one of these is needed, say so in the report and leave it.

## 3. Interfaces (signatures, not prose)

New: `TestAR52_1_MakefilePythonTakesB` and `TestAR52_2_TestsPythonTakeB` in `internal/packet/ar52_test.go`. Changed: four Makefile recipe lines; three or four test lines that start python. Unchanged: every exported name, every ledger key, every stop reason, the names and bodies of the edited tests apart from the python argument lists, `scripts/`.

## 4. Findings that name this row

`Makefile:2` (`PY :=`), `:16` (`check`), `:30` (`doclint`), `:33` (`hygiene`), `:35` (`validate`). `internal/night/ar17c_test.go:73-96` (`boundedPython`, `runBounded`). `internal/night/ar48_test.go:79` (no `-B`), `:254`, `:346`, `:646`, `:708` (already `py, "-B"`), `:457-459` (`argv`). `internal/packet/ar18_test.go:27-48`. `internal/packet/copyin_test.go:16` (`copyIn`, how the package reaches the repo root). `design/pin-pycache-r1.md:47`. `NICE_TO_HAVE.md`, the item under "The gsent night's unrun gate".

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each of the four acceptance lines with its count, `go vet`'s exit, `make -n doclint`'s line, `make doclint`'s exit and `make hygiene`'s last line. Bullets: each act's red line on `main`; the lines changed, by file and number; any python launch found under another name; what was left. Last line: branch, head sha.
