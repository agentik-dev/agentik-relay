package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ar38Queue is a one-row queue whose defaults end with extra.
func ar38Queue(t *testing.T, extra string) (*Queue, error) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "design", "packets"), 0o755)
	src, err := os.ReadFile(filepath.Join("..", "packet", "testdata", "a3-machine.md"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "design", "packets", "a3-machine.md"), src, 0o644)
	y := "version: 1\ndefaults: {model: m, turns: 10, context: 1000, usd: 5, wall: 1h, merge: branch-only, device: never" +
		extra + "}\nrows:\n  - {id: a3-machine, packet: design/packets/a3-machine.md}\n"
	if err := os.WriteFile(filepath.Join(dir, "queue.yaml"), []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(dir, "queue.yaml")
}

// TestAR38_1_QueueMeasureSeconds (relay-r1 §3, §15; AR38 act 1): the
// defaults' `measure_seconds` parses as an integer from 60 to 14400; 0, 59,
// 14401 and a string are refused, each naming the key; unset reads 0.
func TestAR38_1_QueueMeasureSeconds(t *testing.T) {
	q, err := ar38Queue(t, ", measure_seconds: 3000")
	if err != nil {
		t.Fatal(err)
	}
	if ps := q.Validate(); len(ps) != 0 || q.Defaults.MeasureSeconds != 3000 {
		t.Fatalf("measure_seconds %d, problems %v", q.Defaults.MeasureSeconds, ps)
	}
	for _, v := range []string{"60", "14400"} {
		if q, err := ar38Queue(t, ", measure_seconds: "+v); err != nil || q.Defaults.MeasureSeconds == 0 {
			t.Fatalf("measure_seconds %s: %v", v, err)
		}
	}
	if q, err := ar38Queue(t, ""); err != nil || q.Defaults.MeasureSeconds != 0 {
		t.Fatalf("unset: %v, %+v", err, q)
	}
	for _, v := range []string{"0", "59", "14401", "ten"} {
		_, err := ar38Queue(t, ", measure_seconds: "+v)
		if err == nil || !strings.Contains(err.Error(), "measure_seconds") {
			t.Fatalf("measure_seconds %s: %v; want a refusal naming the key", v, err)
		}
	}
	// A row's own measure_seconds is not a row key: the budget is the plan's.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "queue.yaml"), []byte("version: 1\nrows:\n  - {id: a, measure_seconds: 3000}\n"), 0o600)
	if _, err := Load(dir, "queue.yaml"); err == nil || !strings.Contains(err.Error(), "measure_seconds") {
		t.Fatalf("a row's measure_seconds: %v", err)
	}
}
