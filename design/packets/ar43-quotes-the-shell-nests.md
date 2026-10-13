# Packet AR43: the readers end a quote where the shell ends it

| item | value |
|---|---|
| from | `design/relay-r1.md` §10 (the sitting guards read every command a line runs) and §15 (`packet.Unbounded` is an allowlist); AR42's review round 1 (F1, F2 and the `>&2x` observation); the observation below |
| pin | agent-relay `main` at 23bae2c plus this packet's own commit; worktree `.claude/worktrees/ar43-quotes-the-shell-nests`, branch `ar43-quotes-the-shell-nests` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state. A shell probe runs only `printf`, `echo` or `true`, in a temp dir |
| may touch | `internal/hooks/guards.go`, `internal/hooks/*_test.go`, `internal/packet/packet.go`, `internal/packet/*_test.go`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/hooks -run "^TestAR43_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR43_"` reads 3; `go test ./internal/packet -run "^TestAR43_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR43_"` reads 1 |
| budget | 100 turns, 140k context, `claude-opus-5-5`; one commit per act; `make check` in the foreground, then the last commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`internal/hooks/guards.go:1877-1899`: `readHeredoc` skips every `'`, `"` and `\` and ends the delimiter at the first blank or operator, so a quoted delimiter that holds a blank ends early. `internal/hooks/guards.go` `read` and `internal/packet/packet.go` `redirections` end a double-quoted span at the next unescaped `"`, with no reading of a `${…}` inside it. `internal/packet/packet.go` `redirections`, the `>&` branch: the digits after `>&` are read as an fd and the byte after them is not looked at.

What was seen on 2026-09-30, on `main` at 2c42628 (`Verdict(cmd, true)` in a throwaway test; the shells are this Mac's `/bin/sh` and `/bin/bash` (bash 3.2), `/bin/zsh` and `/bin/dash`, each run with `echo RAN` in place of the hidden command):

| line | the guard | the shells |
|---|---|---|
| `cat <<'a b' ; git push origin main #'` | not denied | all run the second command: the delimiter is `a b`, and `#'` is a comment |
| `cat <<"a b" ; git push origin main #"` | not denied | the same |
| `bash -c 'cat <<"a b" ; git push origin main #"'` | not denied | the same |
| `cat <<'a b' ; relay run #'` | not denied | the same |
| `echo "${x:-"'"}" ; git push origin main #'` | not denied | all four print `'` and run the second command: a `"` inside `"${…}"` nests |
| `echo "${x:-'}" ; echo '}" ; git push origin main` | not denied | bash 3.2 runs the push (a `'` inside `"${…}"` opens a quote); zsh and dash stop on an unmatched quote |
| `cat <<'ab' ; git push origin main #'` | denied, `push` | |

`packet.Unbounded` returns "" for `cat <<'a b' ; make check #'`, for `echo "${x:-"'"}" ; make check #'` and for `echo >&2x`; the last writes a file named `2x`. The relay runs an acceptance line with `pin.Sh -c` (`internal/effects/shell.go:119`).

## 2. Rulings

Standing rules. Work in your worktree on branch `ar43-quotes-the-shell-nests` off `main`. The charter is `history/sittings/2026-09-30-10-ar43.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is. Where the shells differ, the reader keeps both readings, as `segments` does after `let`, so a line either reading holds is read; a reading that only denies more is never a regression.

1. **A quoted heredoc delimiter runs to its closing quote** (act 1, hooks). In `readHeredoc`, a `'` opens a span that takes every byte up to the next `'` into the delimiter; a `"` opens a span that takes every byte up to the next unescaped `"`; a backslash takes the next byte. Outside a span the delimiter still ends at a blank or an operator. Test `TestAR43_1_QuotedHeredocDelimiter` (hooks): in a sitting the first four lines of the table above are denied (`push`, `push`, `push`, `nested-run` by `hookSays`, and `Verdict` denies each); `cat <<'ab' ; git push origin main #'` is still `push`; `cat <<'a b' ; echo ok #'` and `cat <<a\ b ; echo ok` are not denied.

2. **A quote inside `"${…}"` is read as the shells read it** (act 2, hooks). Inside a double-quoted span, `${` opens an expansion that runs to its matching `}`. Inside the expansion a `"` opens a nested double-quoted span that ends at the next unescaped `"`, and the outer span goes on after the `}`. A `'` inside the expansion is read both ways: as a literal (zsh, dash) and as a single-quoted span to the next `'` (bash 3.2), and both readings are kept. Whether the old reading (the inner `"` ends the outer span) is kept as a third is your call. Test `TestAR43_2_QuoteInExpansion` (hooks): in a sitting, `echo "${x:-"'"}" ; git push origin main #'` is `push`, `echo "${x:-"'"}" ; relay run #'` is `nested-run`, and `echo "${x:-'}" ; echo '}" ; git push origin main` is `push`; `echo "${x:-"a"}"`, `echo "${HOME}/x"` and `echo "${x:-'}"` are not denied.

3. **An acceptance line holds no heredoc, no `${`, and no `>&` to a file** (act 3, packet). With AR42's scan (a byte is judged only when it is outside a single-quoted span), `Unbounded` refuses a `<<` and a `${`, each with its own reason text. In `redirections`, the fd after `>&` must be followed by the end of the line, a blank, or one of `;`, `|`, `&`, `)`; anything else is refused with the existing "`>&` without an fd writes a file" text. A packet under `design/packets/` that the new rule refuses joins `spent`, named in the charter. Test `TestAR43_3_NoHeredocNoExpansion` (packet): refused are `cat <<'a b' ; make check #'`, `echo "${x:-"'"}" ; make check #'`, `echo "${x}"`, `cat <<<x`, `echo >&2x` and `echo hi 2>&1x`; still passing are this packet's own two acceptance lines, `grep -c '<<' f`, `grep -c '${' f`, `wc -l x 2>&1` and `wc -l x 2>&1 | grep -c 1`.

After the acts, §10 gains the two readings (a quoted delimiter runs to its closing quote; a quote inside `"${…}"` nests, and a `'` there is read both ways), and §15's allowlist note gains ruling 3's three refusals.

## 3. Interfaces (signatures, not prose)

No exported signature changes. `readHeredoc(cmd string, i int) (int, heredoc)` keeps its shape. `Unbounded(line string) string` gains two refusals.

## 4. Findings that name this row

`internal/hooks/guards.go:1877-1899` (`readHeredoc`); `internal/hooks/guards.go:1464-1471` (`segments`, the two-readings pattern after `let`); `internal/hooks/guards.go` `read` (the double-quote case); `internal/packet/packet.go` `Unbounded`, `ansiC` and `redirections` (the `>&` branch); `internal/hooks/ar21_3_test.go` (`hookSays` and `Verdict` as the tests use them); `internal/effects/shell.go:119` (unchanged).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: each act's red test on `main`; any packet added to `spent`; what was left. Last line: branch, head sha.
