package effects

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileLedger appends to <dir>/<YYYY-MM-DD>.jsonl, the directory created
// 0700 and each file 0600. Dir is always LedgerDir of the state directory
// (D18): the ledger never enters a repo.
type FileLedger struct {
	Dir   string
	Clock Clock
	Night string

	mu sync.Mutex
}

// NewLedger is the ledger under the state directory.
func NewLedger(state string, clock Clock, night string) *FileLedger {
	return &FileLedger{Dir: LedgerDir(state), Clock: clock, Night: night}
}

func (l *FileLedger) Append(event string, fields map[string]any) error {
	now := l.Clock.Now()
	b, err := encodeLine(now, l.Night, event, fields)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(l.Dir, now.Format("2006-01-02")+".jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Lines reads every day file, oldest first; a line that does not parse is
// skipped (a torn write), never fatal; a line the scanner cannot hold (over
// 1 MiB) is an error naming the file.
func (l *FileLedger) Lines() ([]map[string]any, error) {
	files, err := filepath.Glob(filepath.Join(l.Dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var lines []map[string]any
	for _, p := range files {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(bytes.NewReader(src))
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				lines = append(lines, m)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	return lines, nil
}

// encodeLine is one ledger line: {ts, night, event, ...fields}, every string
// capped at TextCap (the guard behind TestLedgerHoldsNoPrompt).
func encodeLine(now time.Time, night, event string, fields map[string]any) ([]byte, error) {
	m := map[string]any{}
	for k, v := range fields {
		m[k] = capAny(v)
	}
	m["ts"] = now.UTC().Format(time.RFC3339)
	m["night"] = night
	m["event"] = event
	return json.Marshal(m)
}

// capAny caps through JSON: v is marshalled and read back as plain values
// (numbers kept exact), so a named string type or a struct's string field is
// capped like any string. A value that does not marshal is returned as is;
// encodeLine's own Marshal then fails on it.
func capAny(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var x any
	if err := d.Decode(&x); err != nil {
		return v
	}
	return capPlain(x)
}

// capPlain caps every string in a value decoded from JSON.
func capPlain(v any) any {
	switch x := v.(type) {
	case string:
		return capText(x, TextCap)
	case []any:
		for i, e := range x {
			x[i] = capPlain(e)
		}
		return x
	case map[string]any:
		for k, e := range x {
			x[k] = capPlain(e)
		}
		return x
	default:
		return v
	}
}

// Green reports whether a ledger line records row id green: a line whose
// row is id and whose result is "green" (the gate's and the sitting's
// verdict lines carry result; §3's "a green line").
func Green(line map[string]any, id string) bool {
	row, _ := line["row"].(string)
	res, _ := line["result"].(string)
	return row == id && strings.EqualFold(res, "green")
}
