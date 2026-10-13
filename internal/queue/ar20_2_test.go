package queue

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestAR20_2_HarnessListFromQueue (ADV11-4): the
// queue's `harness:` is a list of repo-relative paths, in the defaults and
// per row (a row's replaces the defaults'); unset, it is Makefile and
// scripts. Every make spelling is always added. A path that is absolute
// or climbs out of the repo is refused at parse.
func TestAR20_2_HarnessListFromQueue(t *testing.T) {
	dir := t.TempDir()
	loadSrc := func(src string) (*Queue, error) {
		p := filepath.Join(dir, "queue.yaml")
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		return Load(dir, "queue.yaml")
	}
	q, err := loadSrc("version: 1\ndefaults:\n  harness: [src/app, tests]\nrows:\n" +
		"  - {id: a1, packet: a1.md}\n  - id: a2\n    packet: a2.md\n    harness: [Makefile, tools/]\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Rows[0].Harness; !reflect.DeepEqual(got, []string{"src/app", "tests"}) {
		t.Fatalf("a1 harness %q; want the defaults'", got)
	}
	if got := q.Rows[1].Harness; !reflect.DeepEqual(got, []string{"Makefile", "tools/"}) {
		t.Fatalf("a2 harness %q; want its own", got)
	}
	spell := []string{"GNUmakefile", "makefile", "Makefile"}
	if got := HarnessOf(q.Rows[0].Harness); !reflect.DeepEqual(got, append(spell[:3:3], "src/app", "tests")) {
		t.Fatalf("HarnessOf(a1) = %q", got)
	}
	if got := HarnessOf(q.Rows[1].Harness); !reflect.DeepEqual(got, append(spell[:3:3], "tools")) {
		t.Fatalf("HarnessOf(a2) = %q", got)
	}
	q, err = loadSrc("version: 1\nrows:\n  - {id: a1, packet: a1.md}\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := HarnessOf(q.Rows[0].Harness); !reflect.DeepEqual(got, append(spell[:3:3], "scripts")) {
		t.Fatalf("HarnessOf(unset) = %q; want the make spellings and scripts", got)
	}
	for _, bad := range []string{"/etc", "../x", "a/../../x", "..", "a/.."} {
		src := "version: 1\nrows:\n  - id: a1\n    packet: a1.md\n    harness: [" + bad + "]\n"
		if _, err := loadSrc(src); err == nil || !strings.Contains(err.Error(), "harness") {
			t.Errorf("harness [%s] = %v; want refused at parse", bad, err)
		}
		src = "version: 1\ndefaults:\n  harness: [" + bad + "]\nrows:\n  - {id: a1, packet: a1.md}\n"
		if _, err := loadSrc(src); err == nil || !strings.Contains(err.Error(), "harness") {
			t.Errorf("defaults harness [%s] = %v; want refused at parse", bad, err)
		}
	}
	if _, err := loadSrc("version: 1\nrows:\n  - {id: a1, packet: a1.md, harness: scripts}\n"); err == nil {
		t.Error("harness: scripts (not a list) was accepted")
	}
}
