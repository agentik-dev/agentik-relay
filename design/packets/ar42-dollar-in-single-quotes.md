# Packet AR42: a `$` inside single quotes is not an ANSI-C quote

| item | value |
|---|---|
| from | `design/relay-r1.md` §15 and its AR21 note ("`packet.Unbounded` refuses any `$'` outright, whatever its escapes spell, so an anchored `-run` is written in double quotes"); `NICE_TO_HAVE.md` ("The packet grammar refuses an acceptance line whose single-quoted regex ends in `)$'`"); the observation below |
| pin | agent-relay `main` at a968d3b plus this packet's own commit; worktree `.claude/worktrees/ar42-dollar-in-single-quotes`, branch `ar42-dollar-in-single-quotes` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Tests: temp `HOME` and XDG dirs, in-process. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. `go test ./...` only inside `make check`, in the foreground. No `relay run`, `plan`, `gate`, `resume`, `stop`, `kill` or `clean` against the account's state |
| may touch | `internal/packet/packet.go`, `internal/packet/*_test.go`, `design/relay-r1.md`, `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/packet -run "^TestAR42_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR42_"` reads 2; `go test ./internal/packet -run "^TestAR21_3_" -count=1 -v 2>&1 \| grep -c "^--- PASS: TestAR21_3_"` reads 1 |
| budget | 60 turns, 80k context, `claude-opus-5-5`; one commit for the act; `make check` in the foreground, then commit in the same turn |
| verify | `make check` |

## 1. What the source says (quoted)

`internal/packet/packet.go:189-194`: `Unbounded` opens with `if strings.Contains(line, "$'")` and returns "an ANSI-C quote (`$'...'`) is refused: its escapes hide where it ends". The test is on the two bytes, with no reading of the quotes around them.

What was seen on 2026-09-30. `Unbounded("go test ./internal/packet -run '^(TestA|TestB)$' | grep -c ok")` and `Unbounded("grep -cE '^(a|b)$' f")` both return the ANSI-C refusal. In each line the `$` is the last byte inside a single-quoted word and the `'` after it closes that word: the shell reads a literal `$`, and no ANSI-C quote opens. The hooks' guard reads the same commands correctly (`grep -E '^(a|b)$' f` is allowed in a sitting). Packet writers have worked around it by dropping the anchor or by writing the regex in double quotes.

## 2. Rulings

Standing rules. Work in your worktree on branch `ar42-dollar-in-single-quotes` off `main`. The charter is `history/sittings/2026-09-30-9-ar42.md` in doclint's five-part shape, opened at the act, closed before the last commit, no home path. The act's first test is red on `main`. `make check` is 0 at the last commit, apart from a timing test that fails under load and passes alone three times (name it). `DECISIONS.md` stays as it is.

1. **The refusal reads the quotes.** `Unbounded` refuses a `$'` only when its `$` is outside a single-quoted span. One left-to-right scan decides it, with three states. Unquoted: a backslash takes the next byte with it; `'` opens a single-quoted span; `"` opens a double-quoted span. Single-quoted: every byte is literal up to the next `'`. Double-quoted: a backslash takes the next byte with it, `"` closes the span, and `'` opens nothing. A `$` whose next byte is `'` is refused in the unquoted state and in the double-quoted state, and is literal in the single-quoted state. The scan errs toward refusal: `\$'` in the unquoted state is refused too (the backslash does not excuse it), and an unclosed quote changes nothing about the bytes before it. The refusal's text stays as it is.

   Tests. `TestAR42_1_DollarInSingleQuotes`: these lines pass `Unbounded` (it returns ""): `go test ./internal/packet -run '^(TestA|TestB)$' -count=1`, `grep -cE '^(a|b)$' f`, `grep -c 'x$' f \| wc -l` written with a real pipe, `echo 'a$' 'b$'`. `TestAR42_1_AnsiCStillRefused`: these lines are still refused with the ANSI-C text: `echo 'a'$'b'` (the first span closed, so the `$'` is unquoted), `echo 'a$'$'\''; make check`, `echo \$'a'`, `echo "$'"`, `echo "'"$'x'` (a `'` inside double quotes opens nothing), `echo \'$'x'` (an escaped `'` opens nothing), `echo $'a'`. `TestAR21_3_AnsiCQuote` stays green unchanged.

After the act, the AR21 note in `design/relay-r1.md` reads: `packet.Unbounded` refuses a `$'` whose `$` is outside single quotes, whatever its escapes spell; a `$` that ends a single-quoted word is literal, so `-run '^X$'` is accepted. The `NICE_TO_HAVE.md` line on `)$'` is removed.

## 3. Interfaces (signatures, not prose)

No signature changes. `Unbounded(line string) string` replaces its `strings.Contains` test with the scan; the scan may be an unexported helper in `packet.go`.

## 4. Findings that name this row

`internal/packet/packet.go:189-194` (`Unbounded`'s first test); `internal/packet/packet.go:210-234` (`redirections`, which already reads `'…'` and `"…"` spans the same way; unchanged); `internal/packet/ar21_3_test.go` (unchanged, still green); `design/relay-r1.md` (the AR21 note that names the double-quote workaround).

## 5. Forbidden reads

Other repos; the real state root and everything under the account's `~/.local/state/agent-relay`, the account's claude install, its user-level CLAUDE.md, git config, ssh config, ledgers, transcripts; packets but this; worktrees but yours.

## 6. Report (capped)

The table: each acceptance line's exit and last line. Bullets: the red test on `main`; what was left. Last line: branch, head sha.
