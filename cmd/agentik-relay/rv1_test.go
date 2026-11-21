package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentik-dev/agentik-relay/internal/effects"
)

// rv1Doctor runs doctor --json and text over planFakes with every program
// present, env's keys added to the fake env and args after the verb; the
// checks and the text.
func rv1Doctor(t *testing.T, env map[string]string, setup func(top string), args ...string) ([]check, string) {
	t.Helper()
	planFakes(t, "main")
	oldLook, oldProbe, oldWT := lookPath, claudeProbe, gitWorktree
	t.Cleanup(func() { lookPath, claudeProbe, gitWorktree = oldLook, oldProbe, oldWT })
	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	claudeProbe = func(string) error { return nil }
	gitWorktree = func(string) error { return nil }
	base := getenv
	getenv = func(k string) string {
		if v, ok := env[k]; ok {
			return v
		}
		return base(k)
	}
	if setup != nil {
		top, _ := toplevel()
		setup(top)
	}
	var out, text bytes.Buffer
	var errs bytes.Buffer
	run(append([]string{"doctor", "--json"}, args...), strings.NewReader(""), &out, &errs)
	if strings.Contains(errs.String(), "usage:") { // exitUsage is exitUnwell's 2, so the words tell
		t.Fatalf("doctor --json %q: %s", args, errs.String())
	}
	var rep struct{ Checks []check }
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	run(append([]string{"doctor"}, args...), strings.NewReader(""), &text, &bytes.Buffer{})
	return rep.Checks, text.String()
}

func rv1Row(t *testing.T, checks []check, name string) check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %s row: %+v", name, checks)
	return check{}
}

// rv1PluginRoot is a temp plugin root whose plugin file says v; "" writes
// no file.
func rv1PluginRoot(t *testing.T, v string) string {
	t.Helper()
	root := t.TempDir()
	if v != "" {
		os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o755)
		os.WriteFile(filepath.Join(root, ".claude-plugin", "plugin.json"), []byte(`{"name": "relay", "version": "`+v+`"}`), 0o644)
	}
	return root
}

// TestRV1_1_DoctorBinary (RV1 R3 as review 1's F3 rules it, acceptance 1):
// the binary row against the plugin file under CLAUDE_PLUGIN_ROOT. Stamped
// and equal: ok. Stamped and different: a warning naming both versions and
// both ways to reinstall. Unstamped beside a readable plugin file: ok, saying
// a go install build carries no version and make install stamps it. No root
// known: ok with the binary's version alone.
func TestRV1_1_DoctorBinary(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	const remedy = "reinstall the binary (make install in a clone, or the README's go install line)"
	const unstamped = "relay unstamped, the plugin 0.2.4: a go install build carries no version; make install from a clone stamps it"
	for _, c := range []struct {
		name, bin, file string // file "" writes none; "-" sets no variable
		status, detail  string
	}{
		{"older binary", "0.2.3", "0.2.4", "warn", "relay 0.2.3, the plugin 0.2.4: " + remedy},
		{"newer binary", "0.2.5", "0.2.4", "warn", "relay 0.2.5, the plugin 0.2.4: " + remedy},
		{"equal", "0.2.4", "0.2.4", "ok", "relay 0.2.4, the plugin's"},
		{"unstamped", "unstamped", "0.2.4", "ok", unstamped},
		{"no variable", "0.2.4", "-", "ok", "relay 0.2.4"},
		{"unstamped, no variable", "unstamped", "-", "ok", "relay unstamped"},
		{"no file", "0.2.4", "", "ok", "relay 0.2.4"},
	} {
		t.Run(c.name, func(t *testing.T) {
			version = c.bin
			env := map[string]string{"CLAUDE_PLUGIN_ROOT": ""}
			if c.file != "-" {
				env["CLAUDE_PLUGIN_ROOT"] = rv1PluginRoot(t, c.file)
			}
			checks, text := rv1Doctor(t, env, nil)
			row := rv1Row(t, checks, "binary")
			if row.Status != c.status || row.Detail != c.detail || !row.OK || row.Warn != (c.status == "warn") {
				t.Fatalf("binary row %+v, want %s %q", row, c.status, c.detail)
			}
			if (row.Remedy != nil && *row.Remedy != "make install") || (row.Remedy != nil) != row.Warn {
				t.Errorf("remedy %v", row.Remedy)
			}
			var names []string
			for _, ch := range checks {
				names = append(names, ch.Name)
			}
			if !strings.Contains(strings.Join(names, " "), "breaker binary template config") {
				t.Errorf("the binary row is not before template: %v", names)
			}
			if want := strings.Join(wrapWords(c.detail, 99-18), "\n"+strings.Repeat(" ", 18)); !strings.Contains(text, "binary       "+want+"\n") {
				t.Errorf("text has no binary line %q:\n%s", want, text)
			}
		})
	}
}

// TestRV1_2_DoctorPluginRoot (RV1 R3, review 1 F2): doctor --plugin-root
// names the plugin's root, since the Bash tool's shell holds no
// CLAUDE_PLUGIN_ROOT; the flag wins over the variable, and an empty or
// unreadable root reads as none.
func TestRV1_2_DoctorPluginRoot(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	version = "0.2.4"
	for _, c := range []struct {
		name   string
		env    string // the variable's root's version; "-" sets none
		args   func(t *testing.T) []string
		status string
		detail string
	}{
		{"the flag alone", "-", func(t *testing.T) []string { return []string{"--plugin-root", rv1PluginRoot(t, "0.2.4")} },
			"ok", "relay 0.2.4, the plugin's"},
		{"the flag's = form", "-", func(t *testing.T) []string { return []string{"--plugin-root=" + rv1PluginRoot(t, "0.2.3")} },
			"warn", "relay 0.2.4, the plugin 0.2.3: reinstall the binary (make install in a clone, or the README's go install line)"},
		{"the flag over the variable", "0.2.3", func(t *testing.T) []string { return []string{"--plugin-root", rv1PluginRoot(t, "0.2.4")} },
			"ok", "relay 0.2.4, the plugin's"},
		{"the variable alone", "0.2.3", func(t *testing.T) []string { return nil },
			"warn", "relay 0.2.4, the plugin 0.2.3: reinstall the binary (make install in a clone, or the README's go install line)"},
		{"an empty root", "-", func(t *testing.T) []string { return []string{"--plugin-root", ""} },
			"ok", "relay 0.2.4"},
		{"an unreadable root", "-", func(t *testing.T) []string { return []string{"--plugin-root", rv1PluginRoot(t, "")} },
			"ok", "relay 0.2.4"},
		{"a root that is not there", "-", func(t *testing.T) []string { return []string{"--plugin-root", filepath.Join(t.TempDir(), "gone")} },
			"ok", "relay 0.2.4"},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := map[string]string{"CLAUDE_PLUGIN_ROOT": ""}
			if c.env != "-" {
				env["CLAUDE_PLUGIN_ROOT"] = rv1PluginRoot(t, c.env)
			}
			checks, _ := rv1Doctor(t, env, nil, c.args(t)...)
			if row := rv1Row(t, checks, "binary"); row.Status != c.status || row.Detail != c.detail {
				t.Fatalf("binary row %+v, want %s %q", row, c.status, c.detail)
			}
		})
	}
}

// TestRV1_3_DoctorEnabled (RV1 R4, acceptance 3): a repo file holding only
// the session tool's relay section reads an ok config row.
func TestRV1_3_DoctorEnabled(t *testing.T) {
	checks, _ := rv1Doctor(t, nil, func(top string) {
		p := filepath.Join(top, effects.RepoConfigFile)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("relay: {enabled: auto, queue: q, merge: m}\n"), 0o644)
	})
	row := rv1Row(t, checks, "config")
	if row.Status != "ok" || strings.Contains(row.Detail, "enabled") || !strings.Contains(row.Detail, "no unknown keys") {
		t.Fatalf("config row %+v", row)
	}
}

// TestRV1_4_DescribeEnabled (RV1 R4, acceptance 4): describe config lists
// enabled, saying whose it is, its value and line from the repo file.
func TestRV1_4_DescribeEnabled(t *testing.T) {
	w := ak8Setup(t, true)
	ak8Write(t, w.repoFile(), "relay:\n  enabled: auto\n")
	code, m, _, _ := ak8Run(t, "describe", "config", "--json")
	s := ak8Settings(t, m)["enabled"]
	if code != exitOK || s == nil || s["value"] != "auto" || s["default"] != nil || !strings.Contains(s["summary"].(string), "session tool") {
		t.Fatalf("exit %d, enabled %v", code, s)
	}
	if src := s["source"].(map[string]any); src["layer"] != "repo" || src["line"] != float64(2) {
		t.Errorf("source %v", src)
	}
	if len(m["warnings"].([]any)) != 0 {
		t.Errorf("warnings %v", m["warnings"])
	}
}
