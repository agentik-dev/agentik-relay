package relay

import "testing"

// TestAR17D_0_AccountReasonsListed: usage-limit and auth close §1's list
// (twenty-three), both sitting failures that stop a running night.
func TestAR17D_0_AccountReasonsListed(t *testing.T) {
	// AR22 act 2 adds lock-moved after them (twenty-four), AR25 act 4
	// helper-storm (twenty-five), AR32A act 4 the orchestrated night's
	// five after it (thirty).
	if len(StopReasons) != 30 || StopReasons[25] != Done || StopReasons[29] != QueueInvalid || StopReasons[21] != UsageLimit || StopReasons[22] != Auth || StopReasons[23] != LockMoved || StopReasons[24] != HelperStorm {
		t.Fatalf("%d stop reasons, the last %v", len(StopReasons), StopReasons[len(StopReasons)-2:])
	}
	for _, r := range []StopReason{UsageLimit, Auth} {
		if !listed(r, SittingFailures) || !AccountStop(r) {
			t.Errorf("%s: not a sitting failure or not the account's", r)
		}
		if s, err := NightStep(NightRunning, OnSittingFailed, r); err != nil || s != Stopped(r) {
			t.Errorf("%s: %s, %v", r, s, err)
		}
	}
	if AccountStop(StreamError) {
		t.Errorf("stream-error read as the account's")
	}
}
