package effects

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ar80Script is the band's file in a test: it keeps its argv and stdin
// beside itself and answers as its mode file says.
const ar80Script = `#!/bin/sh
d=$(dirname "$0")
printf '%s\n' "$@" > "$d/argv"
cat > "$d/stdin"
case "$(cat "$d/mode")" in
ok) printf '{"verdict":"not-started","mode":"observe","text":"no room","peak":104.5}\n' ;;
unread) printf '{"verdict":"unread","mode":"enforce","text":"","peak":null}\n' ;;
exit3) echo 'the lock: waited 2 s' >&2; exit 3 ;;
slow) sleep 5 ;;
notjson) echo 'not json' ;;
two) echo '{"verdict":"admitted"} {"verdict":"admitted"}' ;;
esac
`

// TestAR80_1_OSBand (AR80 rulings 1, 7): OSBand runs the file with the
// session on its argv and the request on stdin and reads its one object;
// exit 3, a 200 ms Timeout, `not json` and two objects err. Reset reads
// band.json's resetsAt under the state home, the old folder when the new
// is absent, and errs on none, no resetsAt or a time not RFC 3339. The
// `band` key is pace by default, off, or an error naming its line;
// ResolveBand gives off, absent or the file; a denied hold's line notifies.
func TestAR80_1_OSBand(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, BandTool)
	if err := os.WriteFile(file, []byte(ar80Script), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := func(m string) { os.WriteFile(filepath.Join(dir, "mode"), []byte(m), 0o600) }
	b := OSBand{File: file, Timeout: 200 * time.Millisecond}
	req := BandRequest{Model: "claude-opus-5-5", SubagentType: "relay-row", Background: true, ToolUseID: "2026-10-14-2/r1/1"}

	mode("ok")
	v, err := b.Ask(context.Background(), "sess-1", req)
	if err != nil || v.Verdict != "not-started" || v.Mode != "observe" || v.Text != "no room" || v.Peak == nil || *v.Peak != 104.5 {
		t.Fatalf("the answer %+v, %v", v, err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if string(argv) != "admission\nadmit\n--session\nsess-1\n" {
		t.Fatalf("argv %q", argv)
	}
	if want := `{"model":"claude-opus-5-5","subagentType":"relay-row","background":true,"fork":false,"tool_use_id":"2026-10-14-2/r1/1"}`; string(stdin) != want {
		t.Fatalf("stdin %s, want %s", stdin, want)
	}
	mode("unread")
	if v, err := b.Ask(context.Background(), "sess-1", req); err != nil || v.Verdict != "unread" || v.Peak != nil {
		t.Fatalf("unread: %+v, %v", v, err)
	}
	for _, c := range []struct{ mode, want string }{{"exit3", "exit 3"}, {"slow", "deadline"}, {"notjson", "not one JSON object"}, {"two", "more than one"}} {
		mode(c.mode)
		began := time.Now()
		_, err := b.Ask(context.Background(), "sess-1", req)
		if err == nil || !strings.Contains(err.Error(), c.want) || time.Since(began) > 3*time.Second {
			t.Fatalf("%s: %v after %s, want an error naming %q within 3 s", c.mode, err, time.Since(began), c.want)
		}
	}

	// Reset: the new folder, else the old; none, no resetsAt, not RFC 3339.
	state := t.TempDir()
	env := func(k string) string { return map[string]string{"XDG_STATE_HOME": state, "HOME": "/nowhere"}[k] }
	rb := OSBand{File: file, Env: env}
	if _, err := rb.Reset(); err == nil {
		t.Fatal("Reset with no band.json read none as a time")
	}
	put := func(folder, body string) {
		p := filepath.Join(state, folder, "admission", "band.json")
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte(body), 0o600)
	}
	put(strings.SplitN(oldSessionConfig, "/", 2)[0], `{"resetsAt":"2026-10-14T10:30:00-07:00","used":38}`)
	at, err := rb.Reset()
	if want := time.Date(2026, 10, 14, 17, 30, 0, 0, time.UTC); err != nil || !at.Equal(want) {
		t.Fatalf("the old folder's reset %s, %v; want %s", at, err, want)
	}
	put(BandTool, `{"resetsAt":"2026-10-14T11:30:00.250000-07:00"}`)
	if at, err := rb.Reset(); err != nil || !at.Equal(time.Date(2026, 10, 14, 18, 30, 0, 250e6, time.UTC)) {
		t.Fatalf("the new folder's reset %s, %v", at, err)
	}
	for _, body := range []string{`{}`, `{"resetsAt":null}`, `{"resetsAt":"10:30"}`, `not json`} {
		put(BandTool, body)
		if at, err := rb.Reset(); err == nil {
			t.Fatalf("band.json %s read %s", body, at)
		}
	}

	// The key, beside caffeinate.
	fs := &FakeFS{}
	get := func(k string) string { return map[string]string{"HOME": "/h"}[k] }
	if c, err := LoadConfig(fs, get, "/r"); err != nil || c.Band != BandPace {
		t.Fatalf("the default band %q, %v; want pace", c.Band, err)
	}
	fs.WriteFile(filepath.Join("/h", userConfig), []byte("caffeinate: false\nband: off\n"), 0o600)
	if c, err := LoadConfig(fs, get, "/r"); err != nil || c.Band != BandOff {
		t.Fatalf("band: off read %q, %v", c.Band, err)
	}
	fs.WriteFile(filepath.Join("/h", userConfig), []byte("caffeinate: false\nband: maybe\n"), 0o600)
	if _, err := LoadConfig(fs, get, "/r"); err == nil || !strings.Contains(err.Error(), "line 2: band") {
		t.Fatalf("band: maybe: %v, want an error naming line 2", err)
	}

	// ResolveBand: off, absent, the file.
	if bd, says := ResolveBand(BandOff, env); bd != nil || says != "off" {
		t.Fatalf("off: %v, %q", bd, says)
	}
	t.Setenv("PATH", t.TempDir())
	if bd, says := ResolveBand(BandPace, env); bd != nil || says != "absent" {
		t.Fatalf("absent: %v, %q", bd, says)
	}
	t.Setenv("PATH", dir)
	real, _ := filepath.EvalSymlinks(file)
	bd, says := ResolveBand(BandPace, env)
	if ob, ok := bd.(OSBand); !ok || ob.File != real || says != real || ob.Timeout != BandTimeout {
		t.Fatalf("resolved: %#v, %q; want the OSBand of %s", bd, says, real)
	}

	// The notice: a held denied line only (ruling 7).
	n, ok := Notices("band", map[string]any{"night": "n", "row": "r1", "part": 1, "state": "held", "verdict": "denied", "peak": 131.0, "text": "SECRET"})
	if !ok || n.Priority != 3 || n.Body != "verdict=denied peak=131; move accounts" {
		t.Fatalf("the denied notice %+v, %v", n, ok)
	}
	for _, f := range []map[string]any{{"state": "held", "verdict": "not-started"}, {"state": "admitted", "verdict": "denied"}, {"state": "unchecked"}} {
		if _, ok := Notices("band", f); ok {
			t.Fatalf("%v noticed", f)
		}
	}
}
