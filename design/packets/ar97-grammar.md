# Packet AR97: the packet grammar refuses `:/` and admits `go test -C`; the held line names the lever

| item | value |
|---|---|
| from | agentik-hq `HANDOFF.md` item 8 (S3), placed 2026-10-29 from a private hq's intake of 2026-10-22 §7 (S3), §9 (S4) and §3 (its held line, S4); `DECISIONS.md` entries 2, 14 and 15 (§1); portfolio entries 15, 21 and 23; band-pacing-r1 §2, §6; relay-r1 §15; the code at 1b5f64d |
| pin | `main` at 1b5f64d plus this packet's own commit; worktree `.claude/worktrees/ar97-grammar`, branch `ar97-grammar` |
| wall | this repo only; no push, `--amend`, rebase or network; never build a relay binary by hand, never run one outside `make`'s test harness, never run `relay` or `agentik relay` `run`, `plan`, `gate`, `stop`, `kill`, `clean` or `resume`, never write inside `~/.local/state/agent-relay`; never `claude`. No `make check` (the coordinator runs it), `make test`, `make install`, `make readme`, `go test ./...` or `./cmd/...`: the acceptance lines, `go vet`, `gofmt -l internal`, `make doclint`, `make hygiene`, one at a time at `nice -n 10`. Before the report `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/packet/packet.go`, `internal/packet/ar97_test.go` (new), `internal/effects/band.go` (`BandLever`), `internal/effects/notify.go` (the `band` suffix, its comment), `internal/effects/ar80_test.go` (line 131's body), `internal/night/run.go` (line 468), `internal/night/ar80_test.go` (lines 355 and 382), `internal/night/ar97_test.go` (new), `design/band-pacing-r1.md` (its Status line), `design/relay-r1.md` (§15), `design/packets/ar97-grammar.md`, `history/sittings/*.md`, `history/sessions/NEXT`. Nothing else |
| acceptance | `go test ./internal/night -run '^TestAR97_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR97_'` reads 1 (on 1b5f64d: 0); `go test ./internal/packet -run '^TestAR97_' -count=1 -v 2>&1 \| grep -c -E '^--- PASS: TestAR97_'` reads 3 (on 1b5f64d: 0); `go test ./internal/night -run '^TestAR(80\|81\|85\|87)' -count=1 -timeout 600s 2>&1 \| grep -c -E '^ok '` reads 1 (on 1b5f64d: 1); `go test ./internal/effects -run '^TestAR80_' -count=1 -timeout 300s 2>&1 \| grep -c -E '^ok '` reads 1 (on 1b5f64d: 1); `go test ./internal/packet -run '^TestAR(20_5\|21_\|43_3)' -count=1 -timeout 300s 2>&1 \| grep -c -E '^ok '` reads 1 (on 1b5f64d: 1); `grep -c -F 'band: off in the relay config stops the hold' internal/effects/band.go` reads 1 (on 1b5f64d: 0); `grep -c -F 'HEAD^{/' internal/packet/packet.go` reads 1 (on 1b5f64d: 0); `grep -c -F 'must be the first flag' internal/packet/packet.go` reads 1 (on 1b5f64d: 0); `cat design/band-pacing-r1.md design/relay-r1.md \| grep -c -F 'AR97'` reads 2 (on 1b5f64d: 0) |
| budget | 60 turns, 180k context, `claude-opus-5-5`; stop at 48 turns or 144k (commit the act in flight, report); four acts, one commit each: 1 the tests, red; 2 the grammar; 3 the held line; 4 the docs; the charter opens with doclint's five rows |
| verify | `make check` |

## 1. What the source says (quoted)

- The intake, as hq `HANDOFF.md` item 8 places it: "the packet grammar refuses a `:/<text>` revision operand at `plan` with a line naming the loud form `HEAD^{/<text>}`; §9's `-C <dir>` on `go test`, the directory inside the worktree, rides it (S4)." Its §3: a night of 2026-10-18 held a row `not-started` from 09:50 to the 14:00 reset, peak 180 against the ceiling 97, and the held line named no way out.
- band-pacing-r1 §6: "calibration is the plugin's (admission-r1 P1 to P3), the owner's lever `band: off`." §2 item 3 stands: "The `mode` is recorded, not obeyed: obeying `observe` would never pace."
- At 1b5f64d: `run.go:468` logs `relay %s: the band holds it: %s`, the verdict alone; `notify.go:89-91` ends a `denied` notice `; move accounts`. `gitRead` (`packet.go:488`) never reads an operand; `:509-510` hold no `-C`. `:/<text>` searches every ref (`git log -1 ':/qzvq'` names 08553ef, an older row's commit) and after `--` is a pathspec (`git log -1 -- ':/Zx7wq9nomatch'` exits 0, no line); `HEAD^{/<text>}` searches HEAD's history and exits 128 on no match.
- Entry 2: "Every packet follows relay-r1 §2, its writer's appended notes included: under 9,216 bytes, six sections, and acceptance on the allowlist." Entry 14: "A packet is a small, finishable slice with an acceptance table; about four acts fit one builder." Entry 15: "A gate that could not run is unrun, and unrun is red: unknown is never a pass."

## 2. Rulings

Standing rules: the charter, `history/sittings/<date>-<n>-ar97.md`, opens at act 1, closes before the last commit; no home path; acts 2 to 4 quote their red line; commits by explicit path.

1. **The held line names the lever.** `internal/effects/band.go` gains `const BandLever = "band: off in the relay config stops the hold"` beside `BandOff`. `run.go:468` logs `relay <id>: the band holds it: <verdict> (peak <n>%); <BandLever>`, `<n>` being `strconv.FormatFloat(*v.Peak, 'f', -1, 64)`, and `relay <id>: the band holds it: <verdict>; <BandLever>` when the peak is nil. The `denied` notice's suffix (`notify.go:90`) becomes `"; move accounts, or " + BandLever`, under `NoticeCap`. The queued line, the ledger lines, the hold and the golden are unchanged.
2. **`:/` refused.** `gitRead` reads each word after the verb that does not begin with `-`: `...` and `..` split it, each side drops one leading `^`, and a side that begins with `:/` refuses the line, message M1, the only `HEAD^{/` in `packet.go`. A separate option value (`--grep :/x`) is refused too; `--grep=:/x` passes. `plan`'s problem row comes through `packetErr` (`queue.go:155`), unchanged.
3. **`go test -C`.** `goTest` admits `-C` or `--C`, its value apart or after `=`, only at `k == 0` (else M2), the value matching `^[A-Za-z0-9_.][A-Za-z0-9_./-]*$` with no `/`-separated part `..` (else M3). The off-list message names `-C` first. `go list` and `go vet` stay out: the allowlist's one go verb is `go test`.
4. **Docs, act 4.** band-pacing-r1's Status line, its change log, gains one sentence: "AR97 (2026-10-29): the held line and the `denied` notice name the lever `band: off`." relay-r1 §15 gains one bullet after the AR43 bullet, its head **`:/` refused, `go test -C` admitted (AR97).**, rulings 2 and 3 in two sentences. No other line of either names AR97. README.md and a RUNBOOK do not move: neither lists the refusals.
5. **The tests, act 1.** `internal/night/ar97_test.go`, on `ar80World` with `o.Out = &out`: `TestAR97_1_HeldLineNamesLever` (a `not-started` hold at peak 180 logs `relay r1: the band holds it: not-started (peak 180%); band: off in the relay config stops the hold`; a `wait` with no peak logs `relay r1: the band holds it: wait; band: off in the relay config stops the hold`; a `denied` hold's notice body reads `verdict=denied peak=131; move accounts, or band: off in the relay config stops the hold`, as `TestAR80_8`). `internal/packet/ar97_test.go`: `TestAR97_2_ColonSlashRefused` (`git log -1 ':/a b'`, `git diff :/a HEAD`, `git log HEAD..:/x`, `git log :/x...HEAD`, `git log ^:/x`, `git log -- :/x`, `git log --grep :/x`: each why holds `HEAD^{/`); `TestAR97_3_LoudFormPasses` (`git log -1 'HEAD^{/a b}'`, `git log --grep=:/x`; `Parse` of a packet with a `:/` line errs); `TestAR97_4_GoTestC` (pass `-C sub`, `--C=a/b`, `-C .`; refuse `-C ../x`, `-C a/../..`, `-C /abs`, `-C '$D'`, `-C a -C b`, `-C` after the package).

```
M1 "`git " + args[0] + " " + w + "` names a commit by `:/<text>`, which searches every ref and is a path after `--`: write `HEAD^{/<text>}`"
M2 "`go test -C` must be the first flag (go's own rule)"
M3 "`go test -C " + dir + "` leaves the worktree or names a variable: want a relative path with no `..`"
```

## 3. Interfaces

`effects.BandLever`; the held log line; the `denied` notice body; `gitRead` and `goTest` keep their signatures; no ledger, status or `--json` key moves.

## 4. Findings that name this row

The intake's §3, §7, §9. For NICE, in the report: `--grep :/x` is a false refusal (S4). The projection's over-read is agentik-session's lead (band-pacing-r1 §6), not this row's.

## 5. Forbidden reads

Transcripts; `~/.local/state`, `~/.claude/projects`; the private hygiene list; any other repo.

## 6. Report (capped)

Six lines at most: each act's sha; each acceptance reading; `go vet`, `gofmt -l`; doclint's exit, hygiene's count; the charter's choices; §4's leads. Fork: none, or one.
