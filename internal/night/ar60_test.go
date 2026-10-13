package night_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agentik-dev/agentik-relay/internal/effects"
	"github.com/agentik-dev/agentik-relay/internal/night"
	"github.com/agentik-dev/agentik-relay/internal/relay"
)

// AR60: a night's status says `planning` from the plan's first step, so a
// reader can tell a planning night from no night.

const (
	ar60State = "/state"
	ar60Night = "2026-10-10-1"
	ar60Root  = "/repos/r"
	ar60Start = "Sat Oct 10 19:01:45 2026"
)

// ar60Fx is a fake bundle whose Procs knows this process's start.
func ar60Fx() (*effects.FakeFS, effects.Effects) {
	fs := &effects.FakeFS{}
	procs := &effects.FakeProcs{Starts: map[int]string{os.Getpid(): ar60Start}}
	return fs, effects.Effects{FS: fs, Procs: procs}
}

// ar60Status is a night's status.json path under ar60State.
func ar60Status(id string) string {
	return filepath.Join(night.NightDir(ar60State, id), "status.json")
}

// TestAR60_1_MarkPlanning: the mark is a status of its own, `planning`
// under this relay's pid and start, with its root and no rows, and the
// dashboard's first line says so.
func TestAR60_1_MarkPlanning(t *testing.T) {
	fs, fx := ar60Fx()
	end := night.MarkPlanning(fx, ar60State, ar60Night, ar60Root)
	if end == nil {
		t.Fatal("MarkPlanning returned no end")
	}
	st := night.ReadStatus(fs, ar60State, ar60Night)
	if st.State != string(relay.Planning) || st.PID != os.Getpid() || st.Started != ar60Start || st.Root != ar60Root || st.Night != ar60Night {
		t.Fatalf("mark = %+v", st)
	}
	if st.Rows == nil || len(st.Rows) != 0 || st.Resume == nil || len(st.Resume) != 0 {
		t.Fatalf("rows %v resume %v; want both []", st.Rows, st.Resume)
	}
	var raw map[string]any
	if err := json.Unmarshal(fs.Files[ar60Status(ar60Night)], &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"rows", "resume"} {
		if l, ok := raw[k].([]any); !ok || len(l) != 0 {
			t.Fatalf("status.json %s = %#v; want []", k, raw[k])
		}
	}
	var b bytes.Buffer
	night.PrintStatus(&b, st)
	first, _, _ := strings.Cut(b.String(), "\n")
	if !strings.HasSuffix(first, ": planning; 0 rows") {
		t.Fatalf("first line %q", first)
	}
	t.Log(first)
}

// TestAR60_2_End: the end rewrites a mark still planning under this pid as
// the night's stop, the pid and start kept; it leaves alone a status the
// night itself rewrote, and a second end changes nothing.
func TestAR60_2_End(t *testing.T) {
	fs, fx := ar60Fx()
	end := night.MarkPlanning(fx, ar60State, ar60Night, ar60Root)
	end(relay.QueueInvalid)
	st := night.ReadStatus(fs, ar60State, ar60Night)
	if st.State != "stopped:queue-invalid" || st.Reason != string(relay.QueueInvalid) || !relay.NightTerminal(relay.NightState(st.State)) {
		t.Fatalf("end(queue-invalid) = %+v", st)
	}
	if st.PID != os.Getpid() || st.Started != ar60Start || st.Root != ar60Root {
		t.Fatalf("end lost the relay: %+v", st)
	}
	before := string(fs.Files[ar60Status(ar60Night)])
	end(relay.OwnerKill)
	if after := string(fs.Files[ar60Status(ar60Night)]); after != before {
		t.Fatalf("a second end rewrote the status:\n%s\nto\n%s", before, after)
	}

	const killed = "2026-10-10-2"
	night.MarkPlanning(fx, ar60State, killed, ar60Root)(relay.OwnerKill)
	if st := night.ReadStatus(fs, ar60State, killed); st.State != "stopped:owner-kill" || st.Reason != "owner-kill" || st.PID != os.Getpid() {
		t.Fatalf("end(owner-kill) = %+v", st)
	}

	const opened = "2026-10-10-3"
	end = night.MarkPlanning(fx, ar60State, opened, ar60Root)
	running := []byte(`{"night":"` + opened + `","state":"running","rows":[],"pid":` + strconv.Itoa(os.Getpid()) + `}` + "\n")
	fs.WriteFile(ar60Status(opened), running, 0o600) // Run's first status, after its open
	end(relay.QueueInvalid)
	if got := fs.Files[ar60Status(opened)]; !bytes.Equal(got, running) {
		t.Fatalf("end rewrote an opened night's status: %s", got)
	}
}

// TestAR60_3_ReapDeadPlanning: the next run's Reap removes a planning
// status whose relay is gone, and keeps its directory, so its number stays
// spent (the review's block 1); a live relay's, this process's and a
// running night's stay, and without Procs nothing goes. On a temp
// directory, as the machine's file system.
func TestAR60_3_ReapDeadPlanning(t *testing.T) {
	state := t.TempDir()
	write := func(id, name string, v any) {
		t.Helper()
		dir := night.NightDir(state, id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const deadPID, livePID = 999999, 4242
	status := func(id, s string, pid int, started string) night.Status {
		return night.Status{Night: id, State: s, Rows: []night.RowStatus{}, Resume: []string{}, PID: pid, Started: started, Root: "/repos/r"}
	}
	const dead, live, self, run = "2026-10-10-1", "2026-10-10-2", "2026-10-10-3", "2026-10-10-4"
	write(dead, "status.json", status(dead, "planning", deadPID, "Sat Oct 10 19:01:45 2026"))
	write(dead, night.GroupsFile, map[string]any{"pid": deadPID, "started": "Sat Oct 10 19:01:45 2026", "groups": []any{}})
	write(live, "status.json", status(live, "planning", livePID, "Sat Oct 10 20:00:00 2026"))
	write(self, "status.json", status(self, "planning", os.Getpid(), "Sat Oct 10 21:00:00 2026"))
	write(run, "status.json", status(run, "running", deadPID, "Sat Oct 10 19:01:45 2026"))

	night.Reap(effects.Effects{FS: effects.OSFS{}, Ledger: &effects.FakeLedger{}}, state)
	if _, err := os.Stat(filepath.Join(night.NightDir(state, dead), "status.json")); err != nil {
		t.Fatalf("Reap without Procs removed the dead plan's status: %v", err)
	}

	procs := &effects.FakeProcs{Starts: map[int]string{livePID: "Sat Oct 10 20:00:00 2026"}} // this pid unknown: only the self rule keeps it
	night.Reap(effects.Effects{FS: effects.OSFS{}, Procs: procs, Ledger: &effects.FakeLedger{}}, state)
	if left, err := os.ReadDir(night.NightDir(state, dead)); err != nil || len(left) != 0 {
		t.Fatalf("the dead plan's directory after Reap: %v %v; want it kept and empty", err, left)
	}
	for id, want := range map[string]string{live: "planning", self: "planning", run: "running"} {
		if st := night.ReadStatus(effects.OSFS{}, state, id); st.State != want {
			t.Fatalf("night %s after Reap = %q, want %q kept", id, st.State, want)
		}
	}
	if got := night.Nights(effects.OSFS{}, state); strings.Join(got, " ") != strings.Join([]string{dead, live, self, run}, " ") {
		t.Fatalf("nights after Reap = %v", got)
	}
}

// TestAR60_4_ReportState: a night with no `night-open` line reports no
// state (AR55 F3), titled NotOpened (F4); TakeStatus fills that state, and
// the stop's reason, from the night's status, so the body agrees with the
// title (the review's N1), and leaves an opened night's `not stopped`.
func TestAR60_4_ReportState(t *testing.T) {
	if night.NotOpened != "not opened" {
		t.Fatalf("NotOpened = %q", night.NotOpened)
	}
	plan := []map[string]any{{"event": "plan", "night": ar60Night, "row": "r1"}}
	check := func(r night.Report, field, title, body string) {
		t.Helper()
		if r.State != field {
			t.Fatalf("Report.State = %q, want %q", r.State, field)
		}
		var j map[string]any
		b, _ := json.Marshal(r)
		if json.Unmarshal(b, &j); j["state"] != field {
			t.Fatalf("JSON state = %#v, want %q", j["state"], field)
		}
		md := string(night.Markdown(r, ar60Root, "/home/u"))
		if want := "# Night " + ar60Night + ": " + title + "\n"; !strings.HasPrefix(md, want) {
			t.Fatalf("markdown title %q, want %q", strings.SplitN(md, "\n", 2)[0], want)
		}
		if want := "\n## Stopped\n\n" + body + "\n"; !strings.Contains(md, want) {
			t.Fatalf("markdown wants %q:\n%s", want, md)
		}
	}
	r := night.BuildReport(plan, ar60Night)
	check(r, "", night.NotOpened, "The night has not opened.")

	for _, c := range []struct{ state, reason, body string }{
		{"planning", "", "The night stopped: not yet."},
		{"stopped:queue-invalid", "queue-invalid", "The night stopped: queue-invalid."},
	} {
		r := night.BuildReport(plan, ar60Night)
		r.TakeStatus(night.Status{Night: ar60Night, State: c.state, Reason: c.reason})
		check(r, c.state, c.state, c.body)
		if r.Stopped.Reason != c.reason {
			t.Fatalf("TakeStatus(%s): stopped.reason %q, want %q", c.state, r.Stopped.Reason, c.reason)
		}
	}

	opened := append([]map[string]any{{"event": "night-open", "night": ar60Night, "root": ar60Root}}, plan...)
	r = night.BuildReport(opened, ar60Night)
	r.TakeStatus(night.Status{Night: ar60Night, State: "planning"})
	check(r, night.NotStopped, night.NotStopped, "The night stopped: not yet.")
}

// ar60Ledger writes a night's ledger lines under state, on the night's date.
func ar60Ledger(t *testing.T, state, id string, events ...string) {
	t.Helper()
	day, _ := time.Parse("2006-01-02", id[:10])
	lg := effects.NewLedger(state, effects.FakeClock{T: day.Add(20 * time.Hour)}, id)
	for _, e := range events {
		if err := lg.Append(e, map[string]any{"root": ar60Root}); err != nil {
			t.Fatal(err)
		}
	}
}

// ar60Write writes a night's status.json under state.
func ar60Write(t *testing.T, state string, st night.Status) {
	t.Helper()
	dir := night.NightDir(state, st.Night)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(st)
	if err := os.WriteFile(filepath.Join(dir, "status.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestAR60Fix_1_ReapKeepsNightNumber (the review's block 1, probes
// TestADV_ReapFreesNightName and TestADVA2): run A is killed by SIGKILL in
// its plan, run B's Reap removes A's mark; A's number stays spent, so run C
// is named after B, and the latest night is C.
func TestAR60Fix_1_ReapKeepsNightNumber(t *testing.T) {
	state := t.TempDir()
	now := time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC)
	osfx := effects.Effects{FS: effects.OSFS{}}
	a := night.NightName(effects.OSFS{}, state, now)
	ar60Write(t, state, night.Status{Night: a, State: "planning", Rows: []night.RowStatus{}, Resume: []string{}, PID: 999999, Started: "Sat Oct 10 19:00:00 2026"})
	ar60Ledger(t, state, a, "plan")
	b := night.NightName(effects.OSFS{}, state, now)
	night.MarkPlanning(osfx, state, b, ar60Root)
	night.Reap(effects.Effects{FS: effects.OSFS{}, Procs: &effects.FakeProcs{}, Ledger: &effects.FakeLedger{}}, state) // B's Run
	c := night.NightName(effects.OSFS{}, state, now)
	night.MarkPlanning(osfx, state, c, ar60Root)
	if c == a || c == b {
		t.Fatalf("A=%s B=%s C=%s: Reap freed a spent number", a, b, c)
	}
	if got := night.Latest(effects.OSFS{}, state); got != c {
		t.Fatalf("A=%s B=%s C=%s: Latest = %s, want C", a, b, c, got)
	}
	if _, err := os.Stat(filepath.Join(night.NightDir(state, a), "status.json")); !os.IsNotExist(err) {
		t.Fatalf("A's dead mark stays: %v", err)
	}
	t.Logf("A=%s B=%s C=%s Latest=%s", a, b, c, night.Latest(effects.OSFS{}, state))
}

// TestAR60Fix_2_LatestSkipsUnopened (the review's block 2, the observer's
// ruling): the default night is the newest that is planning or that opened;
// a night that ended before its night-open line, and a dead plan's reaped
// directory the ledger names with no night-open line, are skipped.
func TestAR60Fix_2_LatestSkipsUnopened(t *testing.T) {
	state := t.TempDir()
	const opened, refused, reaped, planning = "2026-10-10-1", "2026-10-10-2", "2026-10-10-3", "2026-10-10-4"
	ar60Write(t, state, night.Status{Night: opened, State: "stopped:queue-empty", Reason: "queue-empty", Rows: []night.RowStatus{}, Resume: []string{}})
	ar60Ledger(t, state, opened, "plan", "night-open", "stop")
	ar60Write(t, state, night.Status{Night: refused, State: "stopped:queue-invalid", Reason: "queue-invalid", Rows: []night.RowStatus{}, Resume: []string{}, PID: os.Getpid()})
	if err := os.MkdirAll(night.NightDir(state, reaped), 0o700); err != nil {
		t.Fatal(err)
	}
	ar60Ledger(t, state, reaped, "plan")
	if got := night.Latest(effects.OSFS{}, state); got != opened {
		t.Fatalf("Latest = %q, want the opened night %s over a refused run and a reaped plan", got, opened)
	}
	night.MarkPlanning(effects.Effects{FS: effects.OSFS{}}, state, planning, ar60Root)
	if got := night.Latest(effects.OSFS{}, state); got != planning {
		t.Fatalf("Latest = %q, want the planning night %s", got, planning)
	}
	if st := night.ReadStatus(effects.OSFS{}, state, refused); st.State != "stopped:queue-invalid" {
		t.Fatalf("the refused night by name = %+v", st)
	}
}
