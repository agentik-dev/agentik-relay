package effects

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNtfyPost(t *testing.T) {
	var mu sync.Mutex
	var path, title, prio, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		path, title, prio, body = r.URL.Path, r.Header.Get("Title"), r.Header.Get("Priority"), string(b)
		mu.Unlock()
	}))
	defer srv.Close()
	n, ok := Notices("stop", map[string]any{"night": "2026-09-22", "reason": "queue-empty", "red": 0})
	if !ok {
		t.Fatal("stop refused")
	}
	if err := (Ntfy{URL: srv.URL + "/", Topic: "night"}).Send(n); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if path != "/night" || title != "relay 2026-09-22 stop" || prio != "4" || body != "reason=queue-empty red=0" {
		t.Fatalf("got path %q title %q priority %q body %q", path, title, prio, body)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer bad.Close()
	if err := (Ntfy{URL: bad.URL, Topic: "t"}).Send(n); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("a 403 did not fail: %v", err)
	}
}

func TestNoticesRefuseAndCap(t *testing.T) {
	for _, e := range []string{"turn", "spawn", "exit", "stage", "plan", "merge"} {
		if _, ok := Notices(e, map[string]any{"row": "a"}); ok {
			t.Errorf("%s produced a notice", e)
		}
	}
	long := strings.Repeat("x", 500)
	n, _ := Notices("sitting", map[string]any{"row": "a", "state": "red", "reason": long, "gate": long,
		"prompt": "SECRET", "session_id": "S"})
	if len(n.Body) > NoticeCap || !strings.HasSuffix(n.Body, "agentik relay resume a") ||
		strings.Contains(n.Body, "SECRET") || strings.Contains(n.Body, "session_id") || n.Priority != 3 {
		t.Fatalf("sitting notice = %+v", n)
	}
	var b strings.Builder
	if err := Multi(LogNotify{&b}, &FakeNotify{}).Send(n); err != nil || strings.Count(b.String(), "\n") != 1 {
		t.Fatalf("Multi: %v, %q", err, b.String())
	}
}

// AR2-17: a row id long enough to push the resume suffix past NoticeCap must
// not hand capText a negative cap.
func TestAR6_NoticesLongRowID(t *testing.T) {
	row := strings.Repeat("r", 300)
	n, ok := Notices("sitting", map[string]any{"row": row, "state": "red", "reason": "gate"})
	if !ok {
		t.Fatal("sitting refused")
	}
	if len(n.Body) > NoticeCap {
		t.Fatalf("body %d bytes, cap %d", len(n.Body), NoticeCap)
	}
}

// TestAR7_NoticeGateObject: the sitting line's gate is an object; its
// notice reads by its result string, gate=green, and nothing else of it.
func TestAR7_NoticeGateObject(t *testing.T) {
	n, ok := Notices("sitting", map[string]any{"row": "r1", "state": "green", "turns": 3,
		"gate": map[string]any{"result": "green", "verify": map[string]any{"line": "make check", "status": "green"}}})
	if !ok || !strings.Contains(n.Body, "gate=green") || strings.Contains(n.Body, "make check") {
		t.Fatalf("notice %q", n.Body)
	}
}

// TestAK8_R2F2_SendErrorHoldsNoTopic (AK8 review 2, R2-F2): a post that
// fails names the server's URL and the reason, with the topic cut, since
// the run prints the error (`relay: notify <event>: <err>`); a URL the
// topic breaks says so without quoting it; a refusal by status reads as
// before.
func TestAK8_R2F2_SendErrorHoldsNoTopic(t *testing.T) {
	const topic = "qxzv-made-up-topic"
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + l.Addr().String()
	l.Close() // a loopback port nothing listens on
	n := Notice{Title: "t", Body: "b", Priority: 3}
	err = Ntfy{URL: closed + "/", Topic: topic}.Send(n)
	t.Logf("unreachable: %v", err)
	if err == nil || strings.Contains(err.Error(), "qxzv") || !strings.HasPrefix(err.Error(), "ntfy: post to "+closed+"/(topic): ") || !strings.Contains(err.Error(), "dial tcp") {
		t.Errorf("unreachable: %v", err)
	}
	err = Ntfy{URL: closed, Topic: "qxzv%zz"}.Send(n)
	t.Logf("bad escape: %v", err)
	if err == nil || strings.Contains(err.Error(), "qxzv") || strings.Contains(err.Error(), "%zz") || !strings.HasPrefix(err.Error(), "ntfy: post to "+closed+"/(topic): ") {
		t.Errorf("bad escape: %v", err)
	}
	// A server that sends the post on to the closed port, the topic in the
	// path: the client's error names the last URL, which is the topic's.
	hop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, closed+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer hop.Close()
	err = Ntfy{URL: hop.URL, Topic: topic}.Send(n)
	t.Logf("redirect: %v", err)
	if err == nil || strings.Contains(err.Error(), "qxzv") {
		t.Errorf("redirect: %v", err)
	}
	deny := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer deny.Close()
	if err := (Ntfy{URL: deny.URL, Topic: topic}).Send(n); err == nil || err.Error() != "ntfy: 403 Forbidden" {
		t.Errorf("403: %v", err)
	}
}
