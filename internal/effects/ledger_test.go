package effects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileLedger(t *testing.T) {
	state := filepath.Join(t.TempDir(), "agent-relay")
	clock := FakeClock{time.Date(2026, 9, 22, 23, 5, 0, 0, time.UTC)}
	l := NewLedger(state, clock, "2026-09-22-1")
	if err := l.Append("plan", map[string]any{"row": "a3", "result_tail": strings.Repeat("x", 900)}); err != nil {
		t.Fatal(err)
	}
	l.Append("gate", map[string]any{"row": "a3", "result": "green"})
	fi, err := os.Stat(LedgerDir(state))
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("ledger dir mode %v, %v", fi, err)
	}
	fi, err = os.Stat(filepath.Join(LedgerDir(state), "2026-09-22.jsonl"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("ledger file %v, %v", fi, err)
	}
	lines, err := l.Lines()
	if err != nil || len(lines) != 2 {
		t.Fatalf("Lines = %v, %v", lines, err)
	}
	if lines[0]["event"] != "plan" || lines[0]["night"] != "2026-09-22-1" || lines[0]["ts"] != "2026-09-22T23:05:00Z" {
		t.Fatalf("line = %v", lines[0])
	}
	if n := len(lines[0]["result_tail"].(string)); n != TextCap {
		t.Fatalf("a string field is %d bytes, want the %d cap", n, TextCap)
	}
	if !Green(lines[1], "a3") || Green(lines[0], "a3") || Green(lines[1], "a4") {
		t.Fatal("Green wrong")
	}
}

// TestAR7_LinesOverflowError: a line over the scanner's 1 MiB cap is an
// error naming the file, never a silent stop.
func TestAR7_LinesOverflowError(t *testing.T) {
	l := NewLedger(filepath.Join(t.TempDir(), "agent-relay"), FakeClock{}, "n")
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(l.Dir, "2026-09-23.jsonl")
	big := `{"x":"` + strings.Repeat("y", 1<<20+1-len(`{"x":""}`)) + `"}`
	if len(big) != 1<<20+1 {
		t.Fatalf("line is %d bytes", len(big))
	}
	if err := os.WriteFile(p, []byte(`{"event":"a"}`+"\n"+big+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := l.Lines()
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("Lines = %d lines, %v; want an error naming %s", len(lines), err, p)
	}
}

// TestAR7_CapNamedAndStruct: a named string type and a struct's string field
// are capped at TextCap like a plain string.
func TestAR7_CapNamedAndStruct(t *testing.T) {
	type name string
	type inner struct {
		Text string `json:"text"`
		N    int    `json:"n"`
	}
	long := strings.Repeat("z", TextCap+100)
	b, err := encodeLine(time.Unix(0, 0), "n", "e", map[string]any{
		"named":  name(long),
		"struct": inner{Text: long, N: 12345678901},
		"ptr":    &inner{Text: long},
		"list":   []name{name(long)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), strings.Repeat("z", TextCap+1)) {
		t.Fatalf("an uncapped string in %d bytes: %.200s", len(b), b)
	}
	if !strings.Contains(string(b), `"n":12345678901`) {
		t.Fatalf("the number changed: %.200s", b)
	}
}
