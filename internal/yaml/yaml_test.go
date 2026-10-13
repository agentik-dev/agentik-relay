package yaml

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// plain turns a tree into the shape encoding/json decodes to.
func plain(n Node) any {
	switch n.Kind {
	case List:
		out := []any{}
		for _, v := range n.List {
			out = append(out, plain(v))
		}
		return out
	case Map:
		out := map[string]any{}
		for _, kv := range n.Map {
			out[kv.Key] = plain(kv.Val)
		}
		return out
	}
	return n.Str
}

func corpus(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", dir, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("corpus %s: %v, %d files", dir, err, len(files))
	}
	return files
}

func TestAccept(t *testing.T) {
	for _, f := range corpus(t, "accept") {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, _ := os.ReadFile(f)
			got, err := Parse(src)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			raw, err := os.ReadFile(strings.TrimSuffix(f, ".yaml") + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var want any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plain(got), want) {
				g, _ := json.Marshal(plain(got))
				t.Fatalf("tree = %s\nwant %s", g, raw)
			}
		})
	}
}

func TestRefuse(t *testing.T) {
	files := corpus(t, "refuse")
	if len(files) < 8 {
		t.Fatalf("%d refusals; the packet asks for eight or more", len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, _ := os.ReadFile(f)
			want, err := os.ReadFile(strings.TrimSuffix(f, ".yaml") + ".err")
			if err != nil {
				t.Fatal(err)
			}
			_, err = Parse(src)
			var ye *Error
			if !errors.As(err, &ye) {
				t.Fatalf("Parse = %v; want a *yaml.Error", err)
			}
			if prefix := strings.TrimSpace(string(want)); !strings.HasPrefix(err.Error(), prefix) {
				t.Fatalf("Parse = %q; want prefix %q", err, prefix)
			}
		})
	}
}

func TestOrderAndHelpers(t *testing.T) {
	n, err := Parse([]byte("z: 1\na: [x, y]\nm: {b: true, c: no}\nq: 'x'\n"))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, kv := range n.Map {
		keys = append(keys, kv.Key)
	}
	if strings.Join(keys, ",") != "z,a,m,q" {
		t.Fatalf("keys = %v; want document order", keys)
	}
	if z, _ := n.Get("z"); z.Line != 1 {
		t.Errorf("z line = %d", z.Line)
	}
	if v, ok := mustGet(t, n, "z").Int(); !ok || v != 1 {
		t.Errorf("Int = %d, %v", v, ok)
	}
	if v, ok := mustGet(t, n, "a").Strings(); !ok || strings.Join(v, ",") != "x,y" {
		t.Errorf("Strings = %v, %v", v, ok)
	}
	m := mustGet(t, n, "m")
	if v, ok := mustGet(t, m, "b").Bool(); !ok || !v {
		t.Errorf("Bool b = %v, %v", v, ok)
	}
	if _, ok := mustGet(t, m, "c").Bool(); ok {
		t.Error("Bool accepted no; only true and false are booleans")
	}
	if v, ok := mustGet(t, n, "q").String(); !ok || v != "x" {
		t.Errorf("String = %q, %v", v, ok)
	}
	if _, ok := n.Get("absent"); ok {
		t.Error("Get found an absent key")
	}
	if _, ok := m.Strings(); ok {
		t.Error("Strings accepted a map")
	}
	if _, ok := mustGet(t, n, "a").Int(); ok {
		t.Error("Int accepted a list")
	}
}

func mustGet(t *testing.T, n Node, key string) Node {
	t.Helper()
	v, ok := n.Get(key)
	if !ok {
		t.Fatalf("no key %q", key)
	}
	return v
}

// AR2-17: a closer with no opener is refused on a fresh line and on a flow
// continuation, each with its line number.
func TestAR6_YAMLCloserWithoutOpener(t *testing.T) {
	for _, c := range []struct {
		src  string
		line int
	}{{"a: ]\n", 1}, {"a: [1,\n]]\n", 2}} {
		_, err := Parse([]byte(c.src))
		var e *Error
		if !errors.As(err, &e) || e.Line != c.line || e.Msg != "closer without opener" {
			t.Errorf("%q: err %v, want line %d: closer without opener", c.src, err, c.line)
		}
	}
}
