package night

import (
	"reflect"
	"testing"
)

// TestAR17A_0_StageRecordsBase: Stage writes the base's sha and each
// remote's ls-remote on the stage line (a remote it cannot read,
// `unknown`); StagedFromLedger reads the base's back as a claim only, and
// neither the refs nor the URLs (AR21 act 5: the gate binds them now).
func TestAR17A_0_StageRecordsBase(t *testing.T) {
	fx, g, l, rp := stageFakes(t)
	g.LsRemotes = map[string]map[string]string{"origin": {"refs/heads/main": "m0"}}
	st, err := Stage(fx, rp)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Unlock()
	want := map[string]string{"origin refs/heads/main": "m0"}
	if st.BaseSha != "b5" || !reflect.DeepEqual(st.OriginRefs, want) {
		t.Fatalf("staged base %q refs %v", st.BaseSha, st.OriginRefs)
	}
	back := StagedFromLedger(l.Written(), "a6")
	if back == nil || back.BaseSha != "" || back.BaseClaim != "b5" || back.OriginRefs != nil || back.RemoteURLs != nil {
		t.Fatalf("from the ledger: %+v", back)
	}
	g.Err = map[string]error{"LsRemote": errString("offline")}
	if got := LsRemotes(g, "/r", []string{"origin"}, map[string][]string{"origin": {"origin"}}); !reflect.DeepEqual(got, map[string]string{"origin": OriginUnknown}) {
		t.Fatalf("LsRemotes offline = %v", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
