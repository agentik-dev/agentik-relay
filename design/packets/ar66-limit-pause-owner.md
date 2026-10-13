# Packet AR66: the owner, the park and the clean while paused

| item | value |
|---|---|
| from | `design/usage-limit-resume-r1.md` §5 packet 3, its second half (§2's owner while paused, `redSittings`, `clean --merged`, the pause's notice; §4 test 3); `NICE_TO_HAVE.md`'s verify-round note and `ar64-r1`; `design/relay-r1.md` §1, §5, §9 |
| pin | agent-relay `main` after AR65's merge (it moves `night.go`); worktree `.claude/worktrees/ar66-limit-pause-owner`, branch `ar66-limit-pause-owner` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` verb against the account's state: a night may be live. No `make check`, `make readme`, `make e2e` or `go test ./...`: the acceptance lines and package-scoped tests, `go vet` of the three packages, `make doclint` and `make hygiene`, one at a time at `nice -n 10`; every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Hygiene's `lan` rule matches the Go name of the local zone (`time`, then `.Local`): never spell it, in code or prose; take a time's `.Location()`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `internal/night/night.go` (`checkControl` only), `internal/night/stage.go` (`LiveHolder`, new), `internal/night/ar66_test.go` (new), `internal/effects/notify.go` (`noticeFields` and `Notices`' comment), `internal/effects/ar66_test.go` (new), `cmd/relay/main.go` (`redSittings` and `clean` only), `cmd/relay/ar66_test.go` (new), `design/relay-r1.md` (§1, §5, §9, §11 only), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^Test(AR66_\|AR65_\|Control)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR66_\|ok )"` reads 2; `go test ./cmd/relay -run "^Test(AR66_\|Clean\|AR13_4_\|AR32B_3_\|AR32A_5_\|AR32AFix_)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR66_\|ok )"` reads 3; `go test ./internal/effects -run "^Test(AR66_\|Notices\|AR7_\|AR32A_5_)" -count=1 -timeout 300s -v 2>&1 \| grep -c -E "^(--- PASS: TestAR66_\|ok )"` reads 2; `go test ./cmd/relay -run "^TestAR59_4_Gofmt" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 70 turns, 180k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

The record, §2: "`relay stop` reads `stopped:owner-stop` at the next poll; `relay kill` or a signal, `stopped:owner-kill`; `lock-moved` as today." "`redSittings` [...], `nextSitting`'s red list [...] and `orc.stuck` skip that line, so a recovered night parks nothing". "`clean --merged` keeps a tree whose stage lock has a live holder: four of five limited rows ended at `head_before`, an ancestor of main". The `pause` line's "notice keeps quiet hours". §7 keeps "a kill of a paused row stopping the night". §4 test 3: "`stop`: `owner-stop` in one poll; `kill r1`, `kill *`, SIGINT: `owner-kill`; the row `failed: usage-limit`, unlocked, its tree kept." The verify round: "No packet owns `nextSitting` or `orc.stuck`; name an owner." `ar64-r1`: "night.go:1098 lets a kill of a paused row stop nothing."

The code at `df90422` (AR65 moves `night.go`): `checkControl` steps a kill only for `*` or a running row (`internal/night/night.go:1110`, the note's `:1098`) and a stop only while running (`:1115`); `redSittings` reads every failed `sitting` line red (`cmd/relay/main.go:641-643`); `clean --merged` removes a tree whose branch is an ancestor of main (`:983-984`) whatever holds its stage lock (`Locked`, `internal/night/stage.go:157`; `StaleLock`, `:200`); `noticeFields` (`internal/effects/notify.go:37`) has no `pause`, and priority 4 is the ceiling's, the stop's and a question's (`:61`).

## 2. Rulings

The charter is `history/sittings/<the build date>-<n>-ar66.md` in doclint's five-part shape, opened at act 1, closed before the last commit, no home path. Each act's first test is red on the pin; its red line goes in the commit message.

1. **Act 1, `stop` while paused.** `checkControl` takes the stop while paused too: its `owner-stop` line, `OnOwnerStop` to `stopped:owner-stop`, no draining.
2. **Act 1, a kill of a paused row.** `kill <id>` naming a paused row steps `OnOwnerKill`, as `kill *` does; it has no child, so no `kill` line. A signal stays `onSignal`'s. Each paused row then halts by AR65's ruling 6.
3. **Act 2, `redSittings`.** A `sitting` line whose `limit` is `paused` reads no red, so a recovered night parks nothing; a `stop` line's `resume` still parks a row the night halted.
4. **Act 2, `clean --merged`.** A tree whose branch is an ancestor of main stays while its stage lock has a live holder (`night.LiveHolder`: the lock exists and `StaleLock` reads it held), why `its stage lock is held by pid <pid>`, exit 1; a dead holder's tree goes as today. `--stale` is unchanged.
5. **Act 3, the notice.** `noticeFields` gains `pause`: `rows`, `kind`, `resets_at`, `wake`, `count`; priority 3, so quiet hours keep it. The paused `sitting` line sends none: AR65 owns that (its ruling 3).
6. **Tests.** night, in AR65's world (`TestAR65_3`'s stream and clock, one row): `TestAR66_1_OwnerWhilePaused` (paused, then each of: `stop`, `stopped:owner-stop` at its next poll, never draining; `kill r1`, `kill *`, SIGINT on `Signals`, `stopped:owner-kill`; each: r1 failed `usage-limit`, `night.Locked` false, its tree there, `relay resume r1` in the `stop` line). cmd/relay: `TestAR66_2_PausedSittingParksNothing` (`redSittings` of `o.1`'s failed `usage-limit` `sitting` line with `limit: paused`: none, without it `failed`; in `TestAR32AFix_MergeRefusedParksTheDay`'s world the paused line alone refuses nothing, a `stop` line resuming `o.1` parks); `TestAR66_3_CleanMergedKeepsHeld` (`TestClean`'s world with `TestAR13_4`'s locks, each named by the sha1 of its worktree path as `lockPath` names it: a merged tree whose lock's holder lives is kept, its pid named, exit 1; a dead holder's merged tree removed). effects: `TestAR66_4_PauseNotice` (priority 3; the body holds the five fields and no other key).
7. **The record, in act 3.** relay-r1 §1: the owner while paused; §5: the `pause` notice and `Notices`' list; §9: `clean --merged` keeps a tree a live holder locks; §11: the four tests. The verify-round and `ar64-r1` notes gain `(AR66)`.
8. **Not done.** `nextSitting`'s red list and `orc.stuck` are packet 4's: only its wake cuts a sitting after a pause, and a paused sitting never merges, so `judge` never counts it. The wake, the guards and the readers (packets 4 to 6), the `stop` verb's words for a paused night among them. AR65 and AR66 install only with packet 4.

## 3. Interfaces (signatures, not prose)

`night`: `func LiveHolder(procs effects.Procs, worktree string) (LockHolder, bool)`. Tests `func TestAR66_<n>_<Name>(t *testing.T)`; red on the pin: `_1` (a stop or `kill r1` while paused is ignored: the bounded wait ends), `_2` (the paused line reads red), `_3` (the held tree removed), `_4` (`pause` refused).

## 4. Findings that name this row

`ar64-r1` (AR64 round one): a kill of a paused row stops nothing. The record's §2: four of the five limited rows ended at `head_before`, an ancestor of main, so `clean --merged` would remove a paused row's tree.

## 5. Forbidden reads

Transcripts; the account's relay state directory. Read no other repo.

## 6. Report (capped)

At most 150 words. The table: each acceptance line with its count, `go vet`'s exit on the three packages, `make doclint`'s exit and `make hygiene`'s last line. Bullets: the TestAR66 tests red on the pin and why; each existing test changed; anything filed. Last line: branch, head sha.
