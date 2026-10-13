package night

import "testing"

// TestAR20_1_StageRecordsMain: Stage binds main's sha for a row whose base
// is not main, on the stage line as main_sha; StagedFromLedger reads it
// back never (AR21 act 5: the ledger is a claim), marked FromLedger
// (ADV11-2), its base_sha a claim only.
func TestAR20_1_StageRecordsMain(t *testing.T) {
	fx, _, l, rp := stageFakes(t)
	st, err := Stage(fx, rp)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Unlock()
	if st.MainSha != "m5" || st.BaseSha != "b5" || st.FromLedger {
		t.Fatalf("staged main %q base %q from the ledger %v", st.MainSha, st.BaseSha, st.FromLedger)
	}
	back := StagedFromLedger(l.Written(), "a6")
	if back == nil || back.MainSha != "" || back.BaseSha != "" || back.BaseClaim != "b5" || !back.FromLedger {
		t.Fatalf("from the ledger: %+v", back)
	}
}
