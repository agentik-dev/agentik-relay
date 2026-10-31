package effects

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAR98_2_ResetReadsAnswerResumeAt (AR98 rulings 1, 3, 6): the answer's
// `window` and `resume_at` read through OSBand.Ask, absent or null empty;
// ResumeAfter takes a `resume_at` that parses and is after now, and nothing
// else; Reset keeps its signature and its reading; ReadBandFile reads the
// file Reset finds, its top level's `window` and `demand`.
func TestAR98_2_ResetReadsAnswerResumeAt(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, BandTool)
	if err := os.WriteFile(file, []byte("#!/bin/sh\ncat > /dev/null\ncat \"$(dirname \"$0\")/answer\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := OSBand{File: file, Timeout: 5 * time.Second}
	for _, c := range []struct{ answer, window, resume string }{
		{`{"verdict":"denied","mode":"enforce","text":"x","peak":null,"window":"none","resume_at":"2026-11-02T09:00:00-08:00"}`, "none", "2026-11-02T09:00:00-08:00"},
		{`{"verdict":"wait","window":"five_hour","resume_at":null}`, "five_hour", ""},
		{`{"verdict":"wait","window":null}`, "", ""},
		{`{"verdict":"not-started","mode":"observe","text":"no room","peak":104.5}`, "", ""},
	} {
		if err := os.WriteFile(filepath.Join(dir, "answer"), []byte(c.answer+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		v, err := b.Ask(context.Background(), "sess-1", BandRequest{Model: "m", SubagentType: "relay-row", ToolUseID: "n/r1/1"})
		if err != nil || v.Window != c.window || v.ResumeAt != c.resume {
			t.Fatalf("%s: read %+v, %v; want window %q, resume_at %q", c.answer, v, err, c.window, c.resume)
		}
	}

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	for _, c := range []struct {
		resume string
		ok     bool
	}{
		{later.Format(time.RFC3339), true},
		{"2026-10-30T06:00:00.5-07:00", true},
		{now.Format(time.RFC3339), false},
		{now.Add(-time.Hour).Format(time.RFC3339), false},
		{"", false},
		{"11:00", false},
		{"tomorrow", false},
	} {
		at, ok := BandVerdict{ResumeAt: c.resume}.ResumeAfter(now)
		if ok != c.ok || (ok && !at.After(now)) || (!ok && !at.IsZero()) {
			t.Fatalf("resume_at %q: read %s, %v; want %v", c.resume, at, ok, c.ok)
		}
	}
	if at, ok := (BandVerdict{ResumeAt: later.Format(time.RFC3339)}).ResumeAfter(now); !ok || !at.Equal(later) {
		t.Fatalf("resume_at: read %s, %v; want %s", at, ok, later)
	}

	state := t.TempDir()
	env := func(k string) string { return map[string]string{"XDG_STATE_HOME": state, "HOME": "/nowhere"}[k] }
	put := func(folder, body string) {
		p := filepath.Join(state, folder, "admission", "band.json")
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte(body), 0o600)
	}
	rb := OSBand{File: file, Env: env}
	if _, err := rb.Reset(); err == nil || !strings.Contains(err.Error(), "band.json: none under "+state) {
		t.Fatalf("Reset with no file: %v", err)
	}
	if _, err := ReadBandFile(OSFS{}, env); err == nil {
		t.Fatal("ReadBandFile with no file read one")
	}
	put(strings.SplitN(oldSessionConfig, "/", 2)[0], `{"window":"five_hour","resetsAt":"2026-10-30T10:30:00-07:00"}`)
	f, err := ReadBandFile(OSFS{}, env)
	if err != nil || f.Window != "five_hour" || f.Demand != nil {
		t.Fatalf("the old folder: %+v, %v", f, err)
	}
	put(BandTool, `{"window":"none","demand":{"band":"avoid","until":"2026-10-30T11:00:00-07:00","next":{"band":"free"}}}`)
	f, err = ReadBandFile(OSFS{}, env)
	if err != nil || f.Window != "none" || f.Demand == nil || f.Demand.Band != "avoid" || f.Demand.Until != "2026-10-30T11:00:00-07:00" {
		t.Fatalf("the new folder: %+v, %v", f, err)
	}
	if _, err := rb.Reset(); err == nil || !strings.Contains(err.Error(), "no resetsAt") {
		t.Fatalf("Reset on a `none` file: %v, want no resetsAt", err)
	}
	put(BandTool, `not json`)
	if _, err := ReadBandFile(OSFS{}, env); err == nil {
		t.Fatal("ReadBandFile read a file that is not JSON")
	}
	if _, err := ReadBandFile(OSFS{}, func(string) string { return "" }); err == nil {
		t.Fatal("ReadBandFile read with no state home")
	}
}
