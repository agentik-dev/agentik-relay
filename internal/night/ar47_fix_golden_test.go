package night

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestAR47Fix_GoldenFreezesBreakerKeys (AR47 ruling 4, corrected):
// testdata/json-keys.golden freezes the --json key sets by the types'
// json tags. cmd/agentik-relay's TestJSONShapesFrozen walks night.Plan by
// reflection; it builds no plan, reads no load and no idle share. So a
// key BreakerStats gains is a key the golden gains, whatever `omitempty`
// leaves out of a line: `unconfirmed` is frozen beside `opens`, `wait_s`
// and `load`, and the golden names no breaker key the type lacks.
func TestAR47Fix_GoldenFreezesBreakerKeys(t *testing.T) {
	f, ok := reflect.TypeOf(Plan{}).FieldByName("Breaker")
	if !ok || f.Tag.Get("json") != "breaker" || f.Type != reflect.TypeOf(BreakerStats{}) {
		t.Fatalf("Plan.Breaker %+v", f)
	}
	var want []string
	for i := 0; i < f.Type.NumField(); i++ {
		k := f.Type.Field(i)
		name, _, _ := strings.Cut(k.Tag.Get("json"), ",")
		if !k.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = k.Name
		}
		want = append(want, "plan breaker."+name)
	}
	b, err := os.ReadFile("testdata/json-keys.golden")
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "plan breaker.") {
			have = append(have, l)
		}
	}
	sort.Strings(want)
	sort.Strings(have)
	if fmt.Sprint(have) != fmt.Sprint(want) || !strings.Contains(fmt.Sprint(want), "plan breaker.unconfirmed") {
		t.Fatalf("the golden's breaker keys %v, BreakerStats's %v", have, want)
	}
}
