# Packet sync-rf1: the vendored copies take agentik-commons' rf1 bytes

| item | value |
|---|---|
| from | agentik-commons `NICE_TO_HAVE.md`, the S3 line "rf1 (2026-10-14), the vendored copies": the research fixes changed the canonical `scripts/hygiene` (rf1-3, the `renamed` group's old ids end at a word boundary) and `scripts/bounded` (rf1-1, its docstring states the load rule), and every repo that vendors them still carries the earlier bytes; the research audit of 2026-10-16, its significant remaining items, item 4 |
| pin | `main` at `15a2920`; agentik-commons `main` at `e59a318`; branch `sync-rf1`, worktree `.claude/worktrees/sync-rf1`, off main; a hand-run builder |
| wall | this repo only; agentik-commons read-only (its `scripts/sync` writes here, once). No push, merge, `--amend`, rebase, `git stash`, network or install; no `claude` beyond `make check`'s; never build or run a relay binary or verb outside `make check`'s harness. Never write under `.venv`, `~/.local/state` or `~/.config`; one make at a time on the machine |
| may touch | `scripts/agentik`, `scripts/hygiene`, `scripts/bounded`, `scripts/vendored.sha256` (by agentik-commons' `scripts/sync` only), `design/packets/sync-rf1.md`, `history/sittings/*.md`, `history/sessions/NEXT`, `internal/night/ar57_test.go` (its `boundedCanonical` pin and that pin's comment only). Nothing else |
| acceptance | `grep -c -F -e '4cd780d9c66d996e58b217b39ee0679e426172ab9ad7bd4e17971b188e7e7600  scripts/agentik' -e '8ef611607c0fdc44703fa48c374ebc00edc1488da56292985380200d219eb2e9  scripts/hygiene' -e '188adff23545f99159a189174baf3771f7e790e5f1256958b0e3e3ac4ed11306  scripts/bounded' scripts/vendored.sha256` reads `3` (on main: `1`, the `agentik` pin alone); `git diff --name-only main \| grep -c '^scripts/'` reads `3` (on main: `0`); `grep -c -F '188adff23545f99159a189174baf3771f7e790e5f1256958b0e3e3ac4ed11306' internal/night/ar57_test.go` reads `1` (on main: `0`) |
| budget | 20 turns, 80k context; one helper sitting, checkpoint at 16 turns; one commit per act |
| verify | `make check` |

## 1. What the source says

- agentik-commons rf1 (merged at `588530f`) changed two canonical files. `scripts/hygiene`: the `renamed` group's first pattern now ends with `(?![\w-])`, so an old id is matched as a whole name, never inside a longer word or a URL slug that runs on past it (rf1-3); its docstring says so. `scripts/bounded`: the `--load` help states the load rule as GUIDE.md §5 states it (rf1-1); its code is unchanged. `bin/agentik` did not change.
- This repo vendors the bytes from before rf1: `scripts/vendored.sha256` pins `scripts/hygiene` at `32bd5b63…` and `scripts/bounded` at `4fdfdc1d…`; `scripts/agentik` already equals commons' `bin/agentik` (`4cd780d9…`).
- Commons' `scripts/sync <repo>` copies `bin/agentik` to `scripts/agentik` and `scripts/hygiene` and `scripts/bounded` into `scripts/`, then rewrites `scripts/vendored.sha256` in `shasum -a 256 -c` form. It touches nothing else and never commits; this packet's builder does.
- Byte identity follows from two facts: the acceptance lines show `scripts/vendored.sha256` pins the canonical sums, and the verify row's `make check` runs `shasum -a 256 -c scripts/vendored.sha256`. The gate's allowlist (the relay's `packet.Unbounded`) admits neither `shasum` nor `cmp`, so the builder runs those two by hand and reports them (§6).
- This repo also pins `scripts/bounded`'s sum in a test: `internal/night/ar57_test.go`'s `boundedCanonical` (AR57, the copy is canonical byte for byte) holds `4fdfdc1d…`, so `TestAR57_1_BoundedIsCanonical` is red once the sync lands until the pin moves.

## 2. Rulings

1. The sync is the only writer of the four files; none is edited by hand.
2. `.hygiene-allow` is not edited. If the new `hygiene` finds a hit the old one missed, the check is red: the builder reports the group and its count and leaves the branch as it is. The rf1 change narrows `renamed`, so fewer hits are expected, not more.
3. Commits by explicit path: this packet alone first (the packet copy-in rule), then the files the sync changed, then the test pin.
4. `make check` runs once in the worktree, when no other check runs on the machine. A last stderr line `bounded: stop: lock`, `load` or `wall` is unrun, not red: wait 60 s and run it once more.
5. `boundedCanonical` moves to the new canonical sum, `188adff2…`, and its comment names agentik-commons as the canonical copy's home (it named the session plugin's copy, the canonical in AR57's time). No other line of the test moves.

## 3. Interfaces

None change. `scripts/hygiene` keeps its flags, groups, counts-only output and exit codes (0 clean, 1 hit, 2 usage); only `renamed` matches fewer strings. `scripts/bounded` keeps its flags, its lock, load and wall rules and its exits; only its help text moves. `make check` reads `scripts/vendored.sha256` as before.

## 4. Findings that name this row

The S3 line "rf1 (2026-10-14), the vendored copies" in agentik-commons' `NICE_TO_HAVE.md` is answered when this branch and its five siblings merge; the note is commons' to make, not this repo's. The research audit's item 4 (a URL slug read as an old id in a vendored copy) is answered by the same merges.

## 5. Forbidden reads

Transcripts; anything under `~/.local/state` or `~/.claude/projects`; any other repo, except agentik-commons' `scripts/sync`, `bin/agentik`, `scripts/hygiene`, `scripts/bounded` and `GUIDE.md`.

## 6. Report

At most six lines: the branch head, the files the sync changed, each acceptance line's result, the two hand checks (`shasum -a 256 -c scripts/vendored.sha256` prints three lines ending `: OK`; `cmp` of `scripts/agentik`, `scripts/hygiene` and `scripts/bounded` against agentik-commons' `bin/agentik`, `scripts/hygiene` and `scripts/bounded` exits 0), and `make check`'s result (green; red, with the failing target's last lines; or unrun, on a `bounded: stop:` line).
