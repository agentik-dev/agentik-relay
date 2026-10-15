# Packet AR85: an orchestrator sitting asks the band as `relay-orchestrator`

| item | value |
|---|---|
| from | agentik-session `design/fresh-session-r1.md` §2.5 and §5 (2026-10-15, the owner's answer 6 in agentik-session `OPEN-QUESTIONS.md`); `design/band-pacing-r1.md` §2.2 |
| pin | `main` at 659fc77 plus this packet's own commit; a linked worktree, branch `ar85-orchestrator-ask` |
| wall | this repo only. No push, `--amend`, rebase, device, ssh or network. Never build a `relay` binary by hand or run one outside `make`'s test harness. No `relay` or `agentik relay` verb against the account's state: a night may be live. Never run `agentik-session`. |
| may touch | `internal/night/run.go` (`askBand`'s request alone), `internal/effects/band.go` (`FakeBand`'s record alone), `internal/night/ar85_test.go` (new), `design/band-pacing-r1.md` (§2.2's request sentence only), `NICE_TO_HAVE.md` |
| acceptance | `go test ./internal/night -run '^TestAR85_' -count=1 -timeout 300s -v 2>&1 \| grep -c -E '^--- PASS: TestAR85_'` reads 2; `go test ./internal/night -run 'TestAR80\|TestAR81' -count=1 -timeout 600s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0; `go test ./internal/effects -run 'TestAR80' -count=1 -timeout 120s 2>&1 \| grep -c -E '^(--- )?FAIL'` reads 0 |
| budget | 30 turns, 120k context, `claude-opus-5-5`; one act, one commit |
| verify | `make check` |

## 1. What the source says (quoted)

fresh-session-r1 §2.5: "An orchestrator sitting asks as `relay-row` too (band-pacing-r1 §2.2), and it may run to its 6-hour wall. It should ask as `relay-orchestrator`, which (1) leaves to today's projection: a main thread to the reset (§5)." §5: "An orchestrator sitting is under-read at its own ask until it asks as `relay-orchestrator` (§2.5): a·(T − L_row), up to 35-45 points at a fresh window."

band-pacing-r1 §2.2: "a row's first part or orchestrator sitting runs `agentik-session admission admit --session <id>`, stdin `{"model": <its --model>, "subagentType": "relay-row", "background": true, "fork": false, "tool_use_id": "<night>/<row>/<part>"}`".

## 2. Rulings

1. **The type.** `askBand` (`internal/night/run.go:280`, the request at `:391`) sends `subagentType` `relay-orchestrator` when the part is an orchestrator sitting, `rp.Row != ""` (`plan.go:166-168`, as `RunRow` reads it at `run.go:567-569`), and `relay-row` for every other part. Nothing else in the request changes: model, background, fork and `tool_use_id` stay as they are.
2. **The fake.** `FakeBand` keeps each request beside its record, read by a new `Requests() []BandRequest` under its lock. `Asks()` and its strings do not change, so AR80 and AR81's tests stand as written.
3. **Tests.** `TestAR85_1_BuildRowAsksAsRelayRow`: one build row under `FakeBand`, `admitted`; its one request's `SubagentType` is `relay-row`. `TestAR85_2_OrchestratorSittingAsksAsRelayOrchestrator`: an orchestrated night's standing row's first sitting under `FakeBand`, `admitted`; its request's `SubagentType` is `relay-orchestrator`. Use the existing orchestrated-night fixtures (the AR32A or AR68 tests' world) rather than a new one. `TestAR85_2` is red on the pin.
4. **The record.** band-pacing-r1 §2.2's request sentence names both types: `relay-orchestrator` for an orchestrator sitting (AR85, fresh-session-r1 §2.5), `relay-row` for every other first spawn.

## 3. Interfaces (signatures, not prose)

`effects`: `func (f *FakeBand) Requests() []BandRequest`. No new exported name in `night`, no ledger line, key, event or golden change.

## 4. Findings that name this row

agentik-session fresh-session-r1 §5's second risk. The plugin's packet S32 charges a fresh `relay-row` its life; `relay-orchestrator` keeps today's projection there.

## 5. Forbidden reads

Transcripts; the CLI's project records; the account's relay state directory and the session plugin's. Read no other repo but agentik-session's `design/fresh-session-r1.md`.

## 6. Report (capped)

At most 120 words. The table: each acceptance line's count, `go vet`'s, `gofmt -l`'s and `make doclint`'s exits. Bullets: the red line of `TestAR85_2` on the pin; any ruling beyond §2.
