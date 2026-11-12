# NICE_TO_HAVE: what agentik-relay leaves open that blocks nothing

One row per finding, in agentik-hq design/05 §2's table, held by `scripts/recordlint` (`make doclint`): `id` is `n<k>`, never reused; `level` is S1 to S4, S4 when unsure; `area` a component or a backticked path; `evidence` a `file:line`, sha, record, repro or "not confirmed"; a row leaves when it lands. The items filed before 2026-11-01 are frozen, never edited, in `history/NICE-2026-11-01.md`: its S4 and unlevelled items stay there, grepped by path when a packet touches their area. Of its S1 to S3 items, the two below are open on main (AR99 confirmed them, at main `0063660`); the rest landed, by AR75, AR78, AR82 to AR84, AR87 and AR90 to AR94.

## Open findings

| id | level | area | finding | evidence | from |
|---|---|---|---|---|---|
| n1 | S3 | `internal/night` | `TestAR71_2_ModelPause` races under `-race` now and then; the race report's head is not yet read, so test or product is open | repro 2026-11-01: `go test -race -count=10 -run '^TestAR71_2_ModelPause$' ./internal/night` red once in 50 runs (`race detected during execution of test`); archive line 651 | 2026-10-14, AR84's build |
| n2 | S3 | `internal/stream/limit.go` | re-test `rate_limit_event`'s wire spelling on the current CLI: the relay reads the camel case only | `internal/stream/limit.go:59,62`; `design/usage-limit-resume-r1.md:91` keeps the re-test filed here; archive line 576 | 2026-10-14, research fixes rf1; AR82 |
| n3 | S4 | `internal/hooks/guards.go` | the helper-line allowlist admits no `--effort`, so a row's own `claude -p` helper runs at the Mac's settings-file level, never a named one; AR101 left the guard as it was | `internal/hooks/guards.go:851`; AR101 §4 | 2026-11-11, AR101's build |
