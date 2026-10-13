# Packet AR46: the guard reads a command through a shell parser (guard-parser packet 1)

| item | value |
|---|---|
| from | `DECISIONS.md` entry 8; `design/guard-parser-r1.md` §2, §4, §5, §6 (packet 1) and §8; `design/relay-r1.md` §10 |
| pin | agent-relay `main` at 4aa6291 plus this packet's own commit; worktree `.claude/worktrees/ar46-guard-tree-reader`, branch `ar46-guard-tree-reader` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network: the library comes from the account's Go module cache with `GOPROXY=off`, and `go mod tidy` is never run. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state. A shell probe runs only `printf`, `echo` or `true`, in a temp dir, with `echo RAN` in place of any guarded command |
| may touch | `go.mod`, `go.sum`, `internal/hooks/guards.go`, `internal/hooks/treereader.go`, `internal/hooks/*_test.go`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/hooks -run "^TestAR46_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR46_"` reads 4 |
| budget | 120 turns, 150k context, `claude-opus-5-5`; one commit per act; `make check` in the foreground, then the last commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`DECISIONS.md` entry 8: inside a relay row the guard parses each Bash command with the `mvdan.cc/sh` Go library and judges every command in the syntax tree; a line the parser rejects is denied in a row, and so is a zsh-only spelling it does not know.

`design/guard-parser-r1.md` §2: "`Segments(cmd) [][]string` keeps its contract word for word. The guard table, `Normalise`, `commandWord`, `wordsLiteral`, `stdinShell`, `bgMark` and `unreadMark` are untouched, and `packet.Unbounded` keeps calling `Segments`. Only the body under `segments(cmd, depth)` is replaced." Read §2 whole before act 1. Rule 4 (`LangPOSIX` for a `-c` string) and the zsh precommand modifiers are packet 2, not built here.

§5: `mvdan.cc/sh/v3 v3.14.1`, packages `syntax` and `fileutil` only. `go get` rewrites `go 1.26` to `go 1.26.0`. The module is in the account's download cache.

The spike's prototype is `relaycopy/internal/hooks/treereader.go` (586 lines) in `~/.cache/agent-relay-spikes/guard-parser-2026-09-30.tgz`. You may extract the archive under `$TMPDIR` and read it. It is a starting point and not the answer: it tests `'` alone for rule 3, and `record-summary.txt` lists the thirteen lines whose verdict or guard name it changes.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar46-guard-tree-reader` off `main`. The charter is `history/sittings/<the build date>-<n>-ar46.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is. The old reader (`read`, `span`, `closing`, `matching`, `doubleQuoted`, `readHeredoc`, `nested`, `ansiC` and their helpers) stays in the file, unreached from `segments`; packet 3 deletes it. If `make check` refuses the unreached code, keep it reachable from one test and say so in the report.

A line denied in a row on `main` is still denied in a row after this packet. Its guard name may change to `unreadable` where the design says so; an existing test that pins the old name for such a line is changed to the new name, and the report lists each such edit. No line goes from denied to allowed, and the only benign lines that go from allowed to denied are the over-denials named in ruling 2. Outside a row no verdict changes at all. Review round 1 named two exceptions, both kept: a line `main` denied only as `literal`, because the hand reader could not read a construct the tree reads, is allowed now; and outside a row an unread line is judged by a loose reading that may deny a guarded command's text inside quotes.

1. **The tree reader sits behind `segments`** (act 1). Add the dependency with `GOPROXY=off go get mvdan.cc/sh/v3@v3.14.1` and commit `go.mod` and `go.sum`. Build the reader in `internal/hooks/treereader.go` as §2 describes: the walker over every simple command in the tree, a word as a string, a substitution as the marker `$()` plus its own segments, the re-read strings (a shell's `-c`, `eval`, `env -S`, `find -exec` and its kin) one level deeper to `maxDepth`, and the union of the `LangBash` and `LangZsh` readings. Route `segments()` through it. Test `TestAR46_1_TreeReads`: in a sitting, `echo "$(case a in a) git push origin main;; esac)"` and `echo $(case a in a) git push origin main;; esac)` are `push` and `echo "$(true; case a in a) relay run;; esac)"` is `nested-run` (each is `unreadable` on `main`); `sh -c 'git push origin main'` and `find . -exec git push origin main \;` are `push`; `echo "$(date)"`, `n="$(git log --oneline | wc -l)"` and `for f in a b; do echo "$f"; done` are not denied. Every test of `internal/hooks` and `internal/packet` that was green on `main` is green, apart from the name edits above.

2. **A quote inside `"${…}"` is unreadable** (act 2, rule 3 with both characters). Test `TestAR46_2_QuoteInBraces`: in a sitting, `echo "${x:-'}" ; git push origin main ; echo "'}"` and `echo "${x:-" ; git push origin main ; echo "}"` are `unreadable`; `echo "${x:-'}" ; echo '}" ; git push origin main #'` is denied; `echo "${x:-"'"}" ; git push origin main #'` is still `push`. The over-denials, each `unreadable` in a sitting and not denied outside one: `echo "${x:-'a'}" ; echo ok` and `echo "${x:-"a b"}"`. Not denied in a sitting: `echo "${x:-a}"` and `echo "${x}" 'it' "s"`.

3. **What the reader cannot read is unreadable, and only in a row** (act 3). A parse error in either dialect, after what did parse is judged; a substitution past `maxDepth`; a line over 64 KB, which is not handed to the parser; a node kind the walker does not know; a panic in the parser or the walker, recovered inside the reader. Test `TestAR46_3_FailsClosed`: in a sitting, `echo "$(`, `wc -l x;<` and `cat <<'a b' ; echo ok #'` are `unreadable`; `git push origin main` followed by a line holding only `<` is `push`; a five-deep `$(…)` around `git push origin main` is `unreadable`; a 70 KB line is `unreadable`; through a seam of your choice, a walker that panics and a node kind it does not know each give `unreadable`. Outside a sitting none of these is denied, and `git commit --amend` placed on any of them is still `amend`.

4. **Review round 1** (the fix round). This ruling replaces the zsh corpus, which was not built: a safety check stopped each helper that began to compose its lines, and none worked around it; the table is the owner's question now (the record's §9). Every input redirect (`<`, `<&`, `<>`) is the bare `<` word. Array elements and keys are re-read. Whenever a reading is unread, a loose reading of the line is judged with the trees, in a row and outside one. On a subagent's call outside a row only a recovered panic or the depth bound is denied `unreadable`, as on `main`. Test `TestAR46_4_ReviewRound1`: the reviewers' lines, and the lines `main` over-denied as `literal`.

After the acts, `design/relay-r1.md` §10 says in three sentences that the sitting guards read a command through the parser, under which dialects, and what is unreadable.

## 3. Interfaces (signatures, not prose)

`Segments(cmd string) [][]string`, `Verdict(cmd string, inRow bool) (deny bool, why string)` and `VerdictEnv` keep their signatures and contracts. `segments(cmd string, depth int) [][]string` keeps its signature. `bgMark` and `unreadMark` keep their values. No new exported name.

## 4. Findings that name this row

`internal/hooks/guards.go:1469-1560` (`maxDepth`, `Segments`, `bgMark`, `unreadMark`, `segments`) and the old reader below it; `internal/hooks/ar21_3_test.go`, `ar43_test.go` and `ar44_test.go` (the regression corpus, with `hookSays` in `ar17b_test.go` and `Verdict` as the tests use them); `internal/packet` (`Unbounded` calls `Segments` and is not changed here).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours. The spike archive named in §1 is allowed.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: each act's red test on `main`; every existing test line whose guard name you changed, with the old and new name; the zsh table's size, and any spelling zsh runs that the reader allows; what was left. Last line: branch, head sha.
