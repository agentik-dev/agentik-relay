package prompt

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"
)

// rulesOrchestrator is an orchestrator row's RULES (orchestrator r1 §3):
// today's commit, push, device and soft-ceiling sentences, no act 0,
// helpers in the foreground, and the precedence sentence last.
//
//go:embed rules-orchestrator.txt
var rulesOrchestrator []byte

// orchestratorFormat is an orchestrator sitting's CONTEXT line (§3), less
// ar32c's push and live words; ar32b adds the green branches to merge. It is hashed
// with its RULES and the body line, so its template hash is its own.
const orchestratorFormat = "CONTEXT: relay orchestrator row %s, sitting %s; your working tree is %s on branch %s (off main at %s); " +
	"your brief: %s; charters: %s; the newest charter at main: %s, its Next act: %s; " +
	"your ceilings: %dk context (the relay asks you to stop at %dk and kills past %dk, a runaway), %d turns; " +
	"red or parked rows: %s; green branches to merge: %s; open questions in OPEN-QUESTIONS.md: %d; the owner's words in the queue: %s\n"

// bodyFormat is the one body line (§3); %s is the sections, each quoted.
const bodyFormat = "Continue from the Next act above; read the brief by section: %s, and the act named.\n"

// OrchestratorContext is what an orchestrator sitting's CONTEXT carries.
type OrchestratorContext struct {
	Row, Sitting, Tree, Branch, Main string // Sitting is `<id>.<n>`; Main is main's sha
	Brief, CharterDir                string // the row's prompt; where charters live
	Charter, NextAct                 string // the newest charter at main and its Next act ("" none)
	Red                              []string
	Green                            []string // this night's build rows green on their branches, unmerged (ar32b)
	OpenQuestions                    int
	Words                            string // the queue's words
	Ceiling, Soft, Kill, Turns       int
}

// OrchestratorRules is the orchestrator's RULES as embedded.
func OrchestratorRules() []byte { return append([]byte(nil), rulesOrchestrator...) }

// OrchestratorTemplateHash is the orchestrator's template hash with no block.
func OrchestratorTemplateHash() string { return OrchestratorTemplateHashFor(nil) }

// OrchestratorTemplateHashFor is sha256 hex of the block and its blank line
// (when there is one), the orchestrator's RULES, its CONTEXT format and its
// body format: never a build row's hash.
func OrchestratorTemplateHashFor(block []byte) string {
	h := sha256.New()
	if block != nil {
		h.Write(block)
		h.Write([]byte("\n"))
	}
	h.Write(rulesOrchestrator)
	h.Write([]byte(orchestratorFormat))
	h.Write([]byte(bodyFormat))
	return hex.EncodeToString(h.Sum(nil))
}

// OrchestratorLine is the rendered CONTEXT line, newline included; the
// Next act's whitespace is folded so the line stays one.
func OrchestratorLine(c OrchestratorContext) string {
	act := strings.Join(strings.Fields(c.NextAct), " ")
	return fmt.Sprintf(orchestratorFormat, c.Row, c.Sitting, orNone(c.Tree), orNone(c.Branch), orNone(c.Main),
		orNone(c.Brief), orNone(c.CharterDir), orNone(c.Charter), orNone(act),
		c.Ceiling/1000, c.Soft/1000, c.Kill/1000, c.Turns, orNone(strings.Join(c.Red, ", ")),
		orNone(strings.Join(c.Green, ", ")), c.OpenQuestions,
		orNone(strings.Join(strings.Fields(c.Words), " ")))
}

// BodyLine is the one body line naming the brief's sections, each quoted.
func BodyLine(sections []string) string {
	q := make([]string, len(sections))
	for i, s := range sections {
		q[i] = "`" + s + "`"
	}
	return fmt.Sprintf(bodyFormat, strings.Join(q, ", "))
}

// RenderOrchestrator is an orchestrator sitting's stdin (§3): the Every
// session block and a blank line when there is one, RULES, a blank line,
// CONTEXT, a blank line, the body line. rulesHash is
// OrchestratorTemplateHashFor(block); promptHash is sha256 hex of stdin.
func RenderOrchestrator(c OrchestratorContext, block []byte, sections []string) (stdin []byte, rulesHash, promptHash string) {
	var b strings.Builder
	if block != nil {
		b.Write(block)
		b.WriteString("\n")
	}
	b.Write(rulesOrchestrator)
	b.WriteString("\n")
	b.WriteString(OrchestratorLine(c))
	b.WriteString("\n")
	b.WriteString(BodyLine(sections))
	stdin = []byte(b.String())
	sum := sha256.Sum256(stdin)
	return stdin, OrchestratorTemplateHashFor(block), hex.EncodeToString(sum[:])
}
