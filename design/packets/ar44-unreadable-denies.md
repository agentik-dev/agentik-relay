# Packet AR44: a command the guard cannot read is denied

| item | value |
|---|---|
| from | `design/relay-r1.md` §10 (the sitting guards read every command a line runs); AR43's review round 1 (hooks F1; packet F1, the panic); the observation below |
| pin | agent-relay `main` at 7086e8c plus this packet's own commit; worktree `.claude/worktrees/ar44-unreadable-denies`, branch `ar44-unreadable-denies` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state. A shell probe runs only `printf`, `echo` or `true`, in a temp dir |
| may touch | `internal/hooks/guards.go`, `internal/hooks/hooks.go`, `internal/hooks/*_test.go`, `design/relay-r1.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/hooks -run "^TestAR44_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR44_"` reads 4 |
| budget | 100 turns, 140k context, `claude-opus-5-5`; one commit per act; `make check` in the foreground, then the last commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`internal/hooks/hooks.go:44-49`: `answer` recovers any panic into `empty{}`, "the answer of a hook that has nothing to say or cannot decide". For `PreToolUse` an empty answer lets the tool call through. `internal/hooks/guards.go`, `closing` and `matching`: they find the end of a `$(…)` and of a bracket pair, and read a `"` flat and a `'…'` span across a `)`.

What was seen on 2026-09-30, on `main` at 9b179f2 and again at 7086e8c for the first line (`Verdict(cmd, true)` in a throwaway test; the shells are this Mac's `/bin/sh` (bash 3.2), `/bin/zsh` and `/bin/dash`, each run with `echo RAN` in place of the hidden command):

- `echo "$(echo "'")" ; git push origin main #'` is not denied; every shell prints `'` and runs the second command. AR43's reviewer found the same for `echo "$(echo "${x:-"'"}")" ; git push origin main #'`.
- `Verdict("git push origin main\n<", true)` panicked (`slice bounds out of range [:2] with capacity 1`), fixed in AR43's fix round. Through the hook the panic became an empty answer, so the call was allowed; `/bin/sh` and `/bin/zsh` run the first line and only then stop on the syntax error. The reader has no other known panic, but any future one is the same door.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar44-unreadable-denies` off `main`. The charter is `history/sittings/2026-09-30-11-ar44.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is. Where the shells differ, the reader keeps both readings, as `segments` does after `let`; a reading that only denies more is never a regression, but a benign line this packet lists as not denied stays not denied.

1. **The end of `$(…)` is found as the shell finds it** (act 1). `closing` and `matching` read quotes as AR43's `doubleQuoted` does: inside a `$(…)` the quoting starts fresh, a `'…'` span and a `"…"` span each hide a `)`, a `"` inside the substitution nests inside an outer double-quoted span, a backslash takes the next byte, and a nested `$(…)` or `${…}` is read by the same rule. The command inside is still read as a command in its own right. Test `TestAR44_1_QuoteInSubstitution`: in a sitting, `echo "$(echo "'")" ; git push origin main #'` is `push`; `echo "$(echo "${x:-"'"}")" ; git push origin main #'` is `push`; `echo "$(echo ')')" ; relay run` is `nested-run`; `echo "$(echo "a)b")" ; git push origin main` is `push`; `echo "$(git push origin main)"` is still `push`. Not denied: `echo "$(date)"`, `echo "$(echo "it's")"`, `echo "$(echo "a)b")"` and `n="$(git log --oneline | wc -l)"`.

2. **A panic in the Bash door denies** (act 2). In `PreToolUse` for the `Bash` tool, when at least one guard would have been put to the command (a sitting's guards, or the helper guard for a subagent's call), a panic while the guards read it is answered with a deny whose reason starts `relay: unreadable:` and tells the caller to write the command more simply. With no guard in play the answer stays empty, and every other event (`Stop`, `SessionStart`, `PostToolUse`) and every other tool keeps its empty answer on a panic: a hook never breaks the owner's own session. Test `TestAR44_2_PanicDenies`: through a test seam of your choice (a guard that panics), `answer("PreToolUse", …)` for a Bash call in a sitting returns the deny, `hookSays` reads `unreadable`, the same input with no sitting and no subagent returns the empty answer, and a panicking `Stop` returns the empty answer.

After the acts, §10 gains both rules in two sentences.

## 3. Interfaces (signatures, not prose)

No exported signature changes. `answer(event string, stdin io.Reader, env func(string) string, fx Effects) (out any)` keeps its shape; where the recover for the Bash door sits is yours to choose.

## 4. Findings that name this row

`internal/hooks/hooks.go:38-63` (`Run`, `answer`, the recover); `internal/hooks/guards.go` `preToolUse` (the Bash case, `guards(env)` and the `HelperRelay` append), `closing`, `matching`, and AR43's `doubleQuoted`; `internal/hooks/ar21_3_test.go` and AR43's tests (`hookSays` and `Verdict` as the tests use them).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: each act's red test on `main`; the seam you chose for act 2; what was left. Last line: branch, head sha.
