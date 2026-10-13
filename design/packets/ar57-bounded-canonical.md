# Packet AR57: `scripts/bounded` becomes the canonical copy, byte for byte

| item | value |
|---|---|
| from | `breaker-signal-r1.md` §5 item 3 ("one commit in each plugin repo that carries a copy … which already differs from this repo's") and `NICE_TO_HAVE.md`'s AR52 line on the guardian's missing `-B` (§1) |
| pin | agent-relay `main` at the commit that lands this packet; worktree `.claude/worktrees/ar57-bounded-canonical`, branch `ar57-bounded-canonical` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand and never run one outside `make`'s test harness: a binary without `PidHomeLink` reaches the account's real pid dir. No `relay` verb at all against the account's state: a real night may be live. No `make check` and no `go test ./...`: the acceptance lines, `go vet ./internal/night ./internal/hooks ./internal/packet`, `make doclint` and `make hygiene` only, one at a time. **Python:** every shell line carries `PYTHONDONTWRITEBYTECODE=1`. Before the report, `make doclint` exits 0 and `make hygiene` reads `hits outside allowlist: 0` |
| may touch | `scripts/bounded` (replaced whole by the canonical bytes, never edited), `internal/night/ar57_test.go` (new), `internal/night/ar48_test.go`, `internal/night/ar17c_test.go`, `internal/hooks/ar17b_test.go`, `internal/packet/ar20_test.go` (each only where a test pins the old guardian argv, the old stop text, or the inherited environment), `README.md` (the two `bounded` lines, only if they no longer say what the file does), `NICE_TO_HAVE.md`, `history/sittings/*.md`. Nothing else |
| acceptance | `go test ./internal/night -run "^TestAR57_" -count=1 -timeout 300s -v 2>&1 \| grep -c "^--- PASS: TestAR57_"` reads 3; `go test ./internal/night -run "^TestAR48" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./internal/night -run "^TestAR17C_" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./internal/hooks -run "^TestAR17B_" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1; `go test ./internal/packet -run "^TestAR(20\|52)_" -count=1 -timeout 300s 2>&1 \| tail -1 \| grep -c "^ok"` reads 1 |
| budget | 50 turns, 150k context, `claude-opus-5-5`; one commit per act |
| verify | `make check` |

## 1. What the source says (quoted)

`breaker-signal-r1.md` §5, item 3: "A row writes only its own repo, so the copies elsewhere are separate work: one commit in each plugin repo that carries a copy (… share one copy today, which already differs from this repo's), and the conventions document's section on `bounded`."

`NICE_TO_HAVE.md`, AR52's line: "`scripts/bounded`'s guardian is started as `sys.executable <bounded> --guard` with no `-B` … The guardian's argv should take `-B`; the file is shared with the plugin repos, so it goes with their copies."

The conventions now name one canonical `bounded`, the session plugin's, byte-identical everywhere, sha256 `4fdfdc1df94364d72fdc378acfade979f7be32342c262687931fe49752ac40b2`. It is this repo's file plus three things this repo lacks:
- **ADV11-1:** the guardian reports the command's pgid on a second fd, and the fallback kills that group first.
- **ADV11-2:** bounded puts `BOUNDED_DEPTH` (its own plus one) in the command's environment. A bounded that starts above depth 0 writes `bounded: nested stop: <why>` instead of `bounded: stop: <why>`.
- **The guardian's `-B`.**

Its idle rule (AR48) is byte-identical to this repo's. The bytes are given read-only at the path the builder's prompt names.

## 2. Rulings

1. Copy the given file over `scripts/bounded` and keep its mode (executable). Never edit it here. A defect found in it goes to the report and to `NICE_TO_HAVE.md`; it is fixed in the canonical copy and copied again.
2. The relay's gate reads `^bounded: stop: (lock|load|wall)$` as unrun (`internal/effects/shell.go:261`), so `bounded: nested stop:` already reads as a plain red, which is ADV11-2's intent. No Go code outside tests changes.
3. Under `make check`, the outer bounded sets `BOUNDED_DEPTH=1`, and every test that starts a copy of `bounded` inherits it, so its stop line would read `nested stop`. Each such test removes `BOUNDED_DEPTH` from the child's environment (or sets it to `0`), unless the test is about depth. Run each acceptance line twice, once plain and once with `BOUNDED_DEPTH=1` exported in the shell, and both runs read the same.
4. A test that pins the guardian's old argv (no `-B`, one fd) takes the new shape. No assertion is dropped or loosened.
5. `TestAR57_1_BoundedIsCanonical`: sha256 of `scripts/bounded` equals the constant above. `TestAR57_2_NestedStop`: with `BOUNDED_DEPTH=1` and a lock already held, the last stderr line is `bounded: nested stop: lock` and the exit is 2. `TestAR57_3_GuardianTakesB`: the guardian's argv, read from inside the command (`ps -o args= -p $PPID`), carries `-B`.

## 3. Interfaces (signatures, not prose)

`func TestAR57_1_BoundedIsCanonical(t *testing.T)`, `func TestAR57_2_NestedStop(t *testing.T)`, `func TestAR57_3_GuardianTakesB(t *testing.T)`. Use the existing helpers `boundedPython` and `runBounded` (`internal/night/ar17c_test.go`).

## 4. Findings that name this row

The AR52 line quoted in §1. The observer, 2026-10-04: the session plugin's copy lacked AR48, and this one lacked ADV11. The session plugin's S18 ported AR48 there, two adversaries found nothing blocking, and the decisions and respeak plugins have taken its bytes.

## 5. Forbidden reads

Transcripts; `~/.local/state/agent-relay`. Read no other repo; the given file in §1 is the only outside read.

## 6. Report (capped)

At most 150 words. The table: each acceptance line with its count, plain and under `BOUNDED_DEPTH=1`; `go vet`'s exit; `make doclint`'s exit and `make hygiene`'s last line. Bullets: which TestAR57 tests were red on `main` and why; each existing test changed and the line it changed; anything filed. Last line: branch, head sha.
