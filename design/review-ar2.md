# AR2: the adversarial review of 5814bd2

Three read-only `claude-opus-5-5` reviewers, at most two at once (session-budget rule 8), each
over one slice of `ap1-plugin` at `5814bd2`. A finding counts only once the coordinator
reproduces it: a test that fails before the fix and passes after, or a shell line with its
exit. `internal/hooks/` sits outside the packet's touch set and outside every slice.

## machines-gate

`internal/relay/`, `internal/gate/`, `internal/night/stage.go`, `internal/night/schedule.go`.

Agent: 41 tool uses, 116k tokens, 417 s; 14 findings (1 blocking, 6 high, 7 low).

### AR2-1: a hand gate discards another row's live worktree (fixed 275221c)
blocking. `internal/gate/gate.go:247`: row `gate-a1`'s tree is the temp path `relay gate a1` removes with `--force`. Test `TestAR2_1_HandKeepsAnotherRowsTree` (failed: `wip.txt: no such file`).

### AR2-2: a hand gate runs lines inside a locked sitting's tree (fixed f4eeef5)
high. `internal/gate/gate.go:236`, §8. Test `TestAR2_2_HandRefusesLockedTree`.

### AR2-3: merge-on-green merges into whatever branch the root is on (fixed 0293b1f)
high. `internal/gate/gate.go:289`, §6 ("a root on another branch leaves the row branch-only"). Test `TestAR2_3_MergeNeedsRootOnMain`.

### AR2-4: the gate swallows ledger append errors, green with no gate line (fixed dfbea41)
high. `internal/gate/gate.go:176`. Test `TestAR2_4_LedgerFailureIsNotGreen`.

### AR2-8: Decide passes a lost, unrun or unset check (fixed ca5fb2f)
low (`Gate.Verify` is unset outside tests). `internal/relay/gate.go:63`: the pass statuses are now listed. Test `TestAR2_8_UnrunChecksNeverPass`.

### AR2-10: a dependency that is the row's base satisfies §3 trivially (handover: the owner, design §3)
high. `internal/night/plan.go:231`: `IsAncestor(a, a)` is true. The code follows §3's words; asking for a green line when `d == base` is a design change. Reproduced by the agent, not by the coordinator.

### AR2-11: the verify line is not judged green-or-same-as-main (handover: the owner, design §6 / D22)
low. `internal/gate/gate.go:314`: a verify line red on main too aborts every merge.

### AR2-12: TestOriginMovedStops and TestMergeNeedsGreen assert less than §11 (handover: the next relay row)
high. No test drives a night to `stopped:origin-moved` (`grep 'stopped:origin-moved'` over the tests exits 1), and nothing scans for another `MergeNoCommit` caller.

### AR2-13: a plan-time satisfied edge outranks a dependency red tonight (refused)
high as filed. `internal/night/schedule.go:62`: a dependency green on record or merged into the base stays satisfied; tonight's rerun going red does not unmerge it.

### AR2-14: lows in machines-gate (observation)
Not reproduced by the coordinator: `q.Order()`'s error reads as `blocked` (schedule.go:41); a failed `lsof` reads as no claude (stage.go:48); an ungathered hand gate prints `unrun` with no cause (gate.go:172, the `--json` shape is frozen); skipped exit and origin checks print as `true` (gate.go:61).

## parsers-effects

`internal/yaml/`, `internal/packet/`, `internal/queue/`, `internal/effects/` (all but
`spawn.go`), `internal/prompt/`.

Agent: 40 tool uses, 147k tokens, 415 s; 15 findings (7 high, 8 low). It ran `pkill -f 'sleep 30'` once to reap a child its reproduction leaked.

### AR2-5: a queue validates zero or negative budgets, turning §7's ceilings off (fixed 36c2051)
high. `internal/queue/queue.go:332`. Test `TestAR2_5_BudgetsPositive`.

### AR2-6: a check that backgrounds a child reads unrun and the child survives (fixed bec50cd)
high. `internal/effects/shell.go:20`: `ErrWaitDelay` gave exit -1; the group is now killed after every line. Test `TestAR2_6_ShellReapsBackgroundChild`.

### AR2-7: TestNoPushPath exempts the whole of git.go (fixed f050ed4)
high. `internal/effects/git_test.go:207`: a `Publish` running `git push` in git.go passed; now only the pre-push hook's tokens are exempt. Test `TestNoPushPath`.

### AR2-15: bare sibling names in `may touch` parse as repo-root paths (handover: the packet cutter, the packet grammar)
high. `internal/packet/packet.go:271`: `git_test.go` after `internal/effects/git.go` means the root file. Resolving against the previous span is ambiguous: `.hygiene-allow` and `Makefile` are bare root names too. The packets should write full paths.

### AR2-16: TestTemplateHashRecorded and TestLedgerHoldsNoPrompt check hand-written spawn lines (handover: the next relay row)
high. `internal/prompt/template_test.go:94`, `internal/effects/noprompt_test.go:43`: neither reads the line `night.RunRow` writes. `grep 'rules_hash\|prompt_hash'` over the night and cmd tests exits 1.

### AR2-17: lows in parsers-effects (observation)
Not reproduced by the coordinator: `Notices` panics on a row id of 186 bytes or more (notify.go:70); `Lines` drops the scanner's error on a line over 1 MiB (ledger.go:66); `capAny` leaves named string types and structs uncapped (ledger.go:91); `Status` hides all untracked `.claude/` content (git.go:60); the shell's last line mixes stderr into `reads` (effects.go:25); the budget pattern truncates `$15.50` (packet.go:88); ids are unique only case-sensitively and `..` is an id (queue.go:340); a packet path may leave the repo (queue.go:87); two malformed YAML inputs parse (yaml.go:155).

## spawn-stream-night

`internal/effects/spawn.go`, `internal/stream/`, `internal/night/` (all but the two files
above), `cmd/relay/`.

Agent: 41 tool uses, 160k tokens, 421 s; 9 findings (6 high, 3 low).

### AR2-9: the exit line carries the child's stderr, prompt bytes and home paths (fixed 7ee129b)
high. `internal/night/run.go:173`, §5: `exit 1: SECRET PROMPT BODY from /Users/...`. Now `exit N` only. Test `TestAR2_9_ExitLineHoldsNoStderr`; its `.hygiene-allow` row came in 1c2d8e2. The spawn-failure path (run.go:99, `err.Error()`) is left: it holds the argv path, not stdin.

### AR2-18: `relay kill` sends SIGTERM with no SIGKILL thirty seconds on (handover: the next relay row)
high. `internal/night/night.go:123`, §7: routing the owner kill through RunRow's escalation is a fix beyond this sitting's budget.

### AR2-19: an unrun gate stops the night `no-progress` and skips every row (handover: the owner, design §1)
high. `internal/night/run.go:192`: `unrun` leaves the sitting `exited`; which terminal state it maps to is the machine's to name.

### AR2-20: the spawn line's append error is swallowed, the child spawns anyway (handover: the next relay row)
high. `internal/night/run.go:86`, §5's brackets.

### AR2-21: doctor fails when `claude --help` omits `--max-turns` (handover: the owner, doctor's probe)
high. `cmd/relay/main.go:158`: the A5 lead; a hidden flag needs another probe or a warning, a design call.

### AR2-22: the end-to-end night never merges (handover: the next relay row)
high. `internal/night/night_test.go:112`, §11 ("asserts ... the merge sha"): `grep 'MergeSHA\|merge-on-green'` over the night and cmd tests exits 1.

### AR2-23: lows in spawn-stream-night (observation)
Not reproduced by the coordinator: `report` and `resume` drop the ledger's read error (main.go:984, 936); the stderr tail can start mid-rune (spawn.go:131); TestPlanEqualsRun passes the plan it compares against.

## Totals

38 agent findings filed as 23 entries: 9 fixed (1 blocking, 7 high, 1 low), 10 handover, 1 refused, and 3 observation entries holding the 16 lows the coordinator did not reproduce. Agents: machines-gate 116k tokens, parsers-effects 147k, spawn-stream-night 160k, all `claude-opus-5-5` with 41, 40 and 41 tool uses; the sitting's whole spend, agents included, was under $9 when this was written.
