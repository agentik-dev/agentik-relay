# agent-relay design: the guard's shell reader (r1)

**Status:** design r1, 2026-09-30, revised once after its one refuter round (§8); packet 1 is next. It is proved by a spike outside the repo (`scratchpad/spike-guard-parser/`, `spike-output.txt`): `mvdan.cc/sh/v3@v3.14.1`, a copy of `internal/hooks` and `internal/packet` at `60935f4` with `segments()` switched to a tree reader, every hidden-command and benign line of `ar21_3`, `ar43*`, `ar44*` and the packet tests run through both readers, each hidden line also run under `/bin/sh` and `/bin/bash` (bash 3.2.57), `/bin/zsh` (5.9) and `/bin/dash` with the command swapped for `echo RAN`. Inputs: `DECISIONS.md` entry 8, relay-r1 §7, §10, §15, the ar43 and ar44 reviews.

## 1. The problem and the decision

**Why a parser at all (2026-10-14, research fixes).** Claude Code's own deny rules do not cover these spellings, by design. "Configure permissions" (code.claude.com, as of 2026-10-14) says a Bash rule matches the invocation Claude usually produces and "isn't a security boundary around the program": `Bash(curl *)` does not stop `/usr/bin/curl …` or `sh -c 'curl …'`, and `Bash(git push *)` does not stop `git -C . push origin main`, `git -c push.default=current push origin main` or `git 'push' origin main`. The absolute-path, `sh -c` and `git -C` spellings are therefore the guard's to read, which is why the hook parses the shell rather than match a prefix. The four helper evasions recorded in `NICE_TO_HAVE.md` (AR49's review, F3: `$(which relay) stop`, `` `which relay` stop ``, `r=relay; $r stop`, `pkill -f 'relay run'`) belong in the guard's test table as rows, denied once F3 lands; no test holds them at this head.

Three review rounds found shapes that hid a `git push` or `relay run` from the hand-written reader in `internal/hooks/guards.go`: a quoted heredoc delimiter, a quote nested in `"${…}"`, a quote inside `$(…)`, a substitution past depth four, a `case` inside a substitution, a reader panic turned into an allow. Each was patched, but the reader is 750 lines of byte-walking that re-derives shell grammar by hand. `DECISIONS.md` entry 8 settles it: inside a relay row the guard parses each Bash command with the `mvdan.cc/sh` Go library and judges every command in the syntax tree; a line the parser rejects is denied in a row, and so is a zsh-only spelling it does not know. This record says where the parser sits and how it meets the hard fact: one parser gives one reading, and the shells on the box differ.

## 2. The design

**Where it sits.** `Segments(cmd) [][]string` keeps its contract word for word. The guard table, `Normalise`, `commandWord`, `wordsLiteral`, `stdinShell`, `bgMark` and `unreadMark` are untouched, and `packet.Unbounded` keeps calling `Segments`. Only the body under `segments(cmd, depth)` is replaced: the hand reader (`read`, `span`, `closing`, `matching`, `doubleQuoted`, `readHeredoc`, `nested`, `ansiC` and helpers) is deleted and rebuilt on `syntax.Parser` plus `syntax.Walk`. A segment is still one simple command as a list of words; a backgrounded segment still carries `bgMark` first; an unreadable line still carries the `unreadMark` segment, which the `unreadable` guard denies in a row and on a subagent's call.

**How a word becomes a string.** The walker visits every `*CallExpr`, `*DeclClause` and `*LetClause` anywhere in the tree (inside `CmdSubst`, `ProcSubst`, subshells, blocks, loops, `case` items, function bodies, redirect words, `let` and `[[ ]]` clauses) and emits its words. A `*Lit` is unescaped as the shell unescapes it; a `*SglQuoted` is its literal text; a `*DblQuoted` is its parts joined; a `*CmdSubst` or `*ProcSubst` becomes the marker `$()` in the word (so a word holding an expansion is never literal) and its statements are emitted as their own segments; `*ParamExp` and `*ArithmExp` keep their source bytes. An assignment is `NAME=value`; an array's elements come back as their own segment, as today.

**Re-read strings.** After each simple command the walker re-reads what the command hands another reader: the `-c` argument of `sh`/`bash`/`zsh`/`dash`/`ksh`; `eval`'s words; `env -S`; each `find -exec`/`-execdir`/`-ok`/`-okdir` command to its `;` or `+`. Each is parsed one level deeper, bounded by `maxDepth` (4) as today; `find -exec` past the bound whose command would itself run a shell is unreadable, as `nested` does today.

**What "unreadable" covers.** A parse error in a required dialect; the depth bound; a line over 64 KB (parsed as nothing, so no input recurses the parser unbounded); a node kind the walker does not know (it fails closed on an unknown `Command`, `WordPart`, `Loop` or arithmetic kind, so a grammar the library grows never slips through); a panic in the parser or the walker, recovered inside the reader; and the dialect divergence below. All become `unreadMark`.

**The dialect rule.** The box speaks two shells a child reaches: the Bash tool runs zsh, `sh -c` / `bash -c` run bash 3.2. The parser follows bash 5.2 (`LangBash`) and zsh 5.9 (`LangZsh`, which the library calls experimental). The rule, proved on the corpus:

1. Parse the top line under `LangBash` and `LangZsh`. Judge the **union** of commands any reading finds, so a command only one grammar exposes is seen.
2. If either required dialect rejects the line, it is unreadable, after the partial trees are judged (a named guard such as `push` answers first on what did parse).
3. A quote character, `'` or `"`, inside a `"${…}"` word makes the line unreadable, whatever the dialect: the divergence a single tree cannot hold (§4), where the library gives one reading and the box's shells split. The spike built the `'` half only (`relaycopy/internal/hooks/treereader.go:389`); the `"` half is this record's rule, and packet 1 proves it red-first (§4, §8).
4. A re-read `-c` string is also parsed under `LangPOSIX`; if the dialects disagree the string is unreadable, as is a construct the box's `-c` shells do not share (`[[ ]]`, arrays, process substitution, `$'…'`, arithmetic commands, a double-`$""`). A `zsh -c` string takes the same rule, although zsh would run those constructs: an over-denial, chosen so that one rule covers every `-c` shell.

**Outside a row nothing changes.** `guards()` adds the row guards only when `inRow`. Outside a row the always-on guards read the union too, but `unreadMark` has no guard, so the owner's session is never denied by the reader. The spike confirms it: five-deep substitutions, a bad parse and a `'`-in-`${…}` line all pass outside a row, while `amend` and `rebase` on the same lines still deny.

## 3. packet.Unbounded

`Unbounded` keeps its pre-scan (`outsideSingle` for `$'`, `<<`, `${`; `redirections`) and keeps calling `Segments`; it does not move onto the tree now. The pre-scan already refuses every construct the tree would reason about (a heredoc, a here-string, a `${…}`, an ANSI-C quote, a substitution), so by the time it calls `Segments` the line is a flat pipeline of plain words, where both readers agree. The spike ran all 42 packet-test lines through both: identical verdicts (208 pass, 212 refused, 0 differ). Moving it onto the tree is a later packet (§6).

## 4. The proof

From `spike-output.txt`. "shells" is which of sh/bash/zsh/dash run the hidden command; "design" is the guard's row answer. A line is marked where the design is wrong or weaker than today.

| line (hidden command shown) | shells that run it | today | design |
|---|---|---|---|
| `echo "${x:-"'"}" ; git push #'` (ar43) | all four | push | push |
| `cat <<'a b' ; git push #'` (ar43) | all four | push | push |
| `echo "$(case a in a) git push;; esac)"` | zsh, dash | unreadable | **push (correct; today over-denies)** |
| `sh -c 'git push'`; `find . -exec git push \;` | all four | push | push |
| `echo $(…×5… git push …)` depth 5 | all four | unreadable | unreadable |
| `echo "${x:-'}" ; git push ; echo "'}"` | zsh, dash | push | **unreadable (rule 3)** |
| `echo "${x:-'}" ; echo '}" ; git push #'` | sh, bash | push | **unreadable (rule 3)** |
| `echo "${x:-" ; git push ; echo "}"` | none | push | **unreadable (rule 3's `"` half; the spike's prototype allowed it)** |
| `repeat 2 git push` (zsh) | zsh | allow | **allow: a hole** |
| `noglob git push`, `nocorrect`, `=git`, `- git` | zsh | allow | **allow: a hole** |
| `alias p='git push'; p` | sh, zsh, dash | allow | **allow: a hole** |
| `echo "$(echo ${x:-)" ; git push ; echo "})"` | sh, bash (3.2) | push | **allow: weaker, see note** |

**The line that decides rule 3.** `echo "${x:-'}" ; git push ; echo "'}"` and `echo "${x:-'}" ; echo '}" ; git push #'` are the same shape with opposite splits: the first runs the push under zsh and dash and not bash 3.2; the second under bash 3.2 and not zsh or dash. The library gives one reading for each: it finds the push in the second and misses it in the first. No one reading covers both, so a `'` inside `"${…}"` must be unreadable.

**Over-denials (benign, denied `unreadable`).** A `'` or `"` inside `"${…}"` on a benign line, an unterminated substitution (`echo "$(`), a bare `<` (`wc -l x;<`), a quoted-delimiter edge (`cat <<'a b' ; echo ok #'`). None runs a hidden command (probe confirms); the row rewrites more simply. The owner's decision, not a defect.

**Holes the design does not close (nor does today).** zsh precommand modifiers (`noglob`, `nocorrect`, `repeat N`, `=`, `-`) parse cleanly and put the real command in argument position, so `git` is not the command word and the push hides; zsh runs them. A shell `alias` then its use defeats any static reader. Pre-existing gaps, not regressions. §6 scopes the modifier fix in; the alias gap is a residual (§7).

**The one weakening, bounded.** `echo "$(echo ${x:-)" ; git push ; echo "})"` runs the push only under bash 3.2, which a child reaches only through `sh -c` / `bash -c`; that nested form is caught by rule 4. At the top level the Bash tool runs zsh, which does not run the push, so the allow is correct for the shell that runs it; today over-denies it. The weakening rests on "Bash tool runs zsh, `-c` runs bash 3.2" (§7).

## 5. The dependency

`mvdan.cc/sh/v3 v3.14.1`, BSD 3-Clause (`LICENSE`: Copyright 2016 Daniel Marti). Two packages compile into the hooks, `syntax` and `fileutil`, both of the one module; no third is built. agent-relay takes this as its first outside dependency (entry 8). `go get` rewrites `go 1.26` to `go 1.26.0` (the module's floor). `go.sum` holds two lines after `go get`; `go mod tidy` pulls the library's test-only modules over the network, so the builder runs `go get mvdan.cc/sh/v3@v3.14.1` and not `tidy`. The spike built and tested `./internal/...` with `GOPROXY=off` from the cache, and again under the pin's env (`GOPROXY=file://<account>/cache/download,off`, `GOSUMDB=off`, fresh `GOMODCACHE`): the module extracts from the account's download cache and the go.sum check passes, so a builder behind the wall builds once the cutting sitting has primed that cache with `go mod download mvdan.cc/sh/v3`. `make check` stays offline: it never fetches, and `GOFLAGS=-mod=readonly` refuses a missing module.

## 6. The cut into packets

Each fits one builder sitting (~100 turns). The regression corpus (`ar21_3`, `ar43*`, `ar44*`, packet `ar42*`/`ar43*`) is the red-first acceptance.

- **Packet 1: the tree reader behind `Segments`, bash plus zsh.** Add the dependency; build `treeSegments` (walker, re-read strings, union of `LangBash` and `LangZsh`, rules 1 to 3 with both quote characters, unknown-node and panic fail-closed, depth and length bounds); route `segments()` through it, old reader left in place and dark. Red-first: every hidden line denied, every benign line allowed, bar the named over-denials, asserted `unreadable`. A zsh-spelling corpus is part of its acceptance (§7's first risk, moved here by the refuter round): each zsh-only spelling with a command in it is read with that command as a segment, or is `unreadable`, never silently neither. Green stays: all of `internal/hooks` and `internal/packet`.
- **Packet 2: the nested `-c` dialect rule and the zsh modifiers.** Rule 4 for re-read strings (`LangPOSIX`, disagreement is unreadable); extend the wrapper table (`commandWord`) with `noglob`, `nocorrect`, `repeat` (one operand), `=` and a leading `-`. Red-first: the modifier lines denied, the `sh -c` divergence line unreadable, ordinary `sh -c 'git push'` still `push`.
- **Packet 3: delete the old reader.** Remove `read`, `span`, `closing`, `matching`, `doubleQuoted`, `readHeredoc`, `ansiC`, `nested`, `substitution`, `background`, `flatDelimiter`, the `reading`/`seen` types and the six-reading loop; nothing calls them after packet 1. Green stays: the full corpus and `make check`.

**Not in scope:** moving `packet.Unbounded` onto the tree (§3); the shell-`alias` gap (§7); the sandbox (AR22); any change to the guard table, `Normalise` or the file door.

## 7. Open risks

- **The library's zsh grammar is incomplete.** A construct it misparses could hide a command or reject a benign line. Settled in packet 1 (§6): a test parsing zsh spellings under `LangZsh`, asserting each is read correctly or `unreadable`, never silently wrong; re-run on each library bump.
- **The weakening rests on which shell runs what.** If the Bash tool ran bash, or `-c` ran a shell the nested rule does not model, the `${x:-)` line would slip. Settle it: a test pinning the top dialect set to `{bash, zsh}` and the `-c` set to `{bash, posix}`, failing on drift, plus `relay doctor` naming the Bash tool's shell.
- **zsh precommand modifiers beyond the five.** The fix is a fixed list; zsh has more. Settle it: a test feeding every zsh precommand word, asserting the command word lands past it or the line is unreadable.
- **The shell `alias` builtin then use.** No static reader follows it; both readers miss it. Settle it: accept as a residual backed by the breaker (AR27C) counting the real process, with a test documenting the gap for a later `alias`-builtin guard.

## 8. The refuter round (2026-09-30, 22:02 to 22:06)

One adversary read this record against `DECISIONS.md`, `internal/hooks/guards.go` and the spike archive, rebuilt the spike's probes offline, and re-ran them on the archive's own corpus. Its verdict: 0 blocking, 1 not blocking.

- **Taken (F1).** Rule 3 said "a quote character" while the prototype tested `'` alone, so the archive's `echo "${x:-" ; git push origin main ; echo "}"` read `today=push design=allowed`, a second line where today denies and the prototype allows, which §4 did not list. No shell runs its push. The record keeps the wider rule: rule 3 now names both characters, §4 lists the line as `unreadable`, and packet 1 builds and tests it.
- **Taken.** The walker's panic recovery and the grammar that governs a `zsh -c` string are now stated in §2. §7's first risk is packet 1's acceptance.
- **Not run (see §9: the builder's zsh table was stopped the same way).** The round's lens was "a line one shell runs that the union rule allows". The adversary's step of composing new candidate lines was stopped by a safety check, and it did not retry, so no new line was tested under any shell or dialect. Nothing may be read from this round about shapes outside the archive's corpus; they are left to packet 1's zsh corpus and to the reviews of the landed code.

## 9. What packet 1's build and review changed (AR46, 2026-09-30)

- **The zsh corpus was not built, so §7's first risk is open again.** A safety check stopped the builder when it began to compose the table, as it had stopped the refuter; neither worked around it and the observer did not re-send them. On every line that exists (1,232 from the spike's corpora and the tests) the tree reader denies what `main` denies in a row. The table is the owner's question in `OPEN-QUESTIONS.md`.
- **§4's one weakening is not in the build.** The builder kept `echo "$(echo ${x:-)" ; git push ; echo "})"` denied, with two rules beyond §2: a comment inside a substitution, and a `${…}` holding `)` inside one, are unreadable. They over-deny ordinary lines (`out=$(` then a comment line; `v=$(echo "${VERSION:-$(cat VERSION)}")`). Packet 2 drops both when rule 4 lands.
- **An unread line is also judged by a loose reading** that is blind to quotes, so a named guard still answers on it, in a row and outside one. Outside a row this can deny a guarded command's text inside quotes on a line the parser rejects.
- **A subagent's call outside a row** is denied `unreadable` only on a recovered panic or the depth bound, as on `main`; §2's "and on a subagent's call" holds in a row only. Otherwise every helper on the box would lose about one ordinary line in eleven.
- **The cost in a row, measured on 598 ordinary lines:** 53 are newly `unreadable`. Nineteen hold a quote inside `"${…}"` (§4). Eleven come from the two substitution rules above. Twenty-three are lines a dialect rejects: `"${var^^}"`, `"${!name}"`, `sleep 1 &; echo x`, `((cd /tmp; ls) | wc -l)`, `ls !(skip).go`, `${VERSION:-"dev"}` in an unquoted heredoc body, a line over 64 KB. Every commit-message heredoc kept its verdict. Thirteen lines `main` over-denied as `literal` are allowed now.
- **The reader's cost:** 3.7 ms for a 16 KB command list and 13.1 ms for 60 KB, against the hand reader's 2.4 ms and 8.7 ms.
