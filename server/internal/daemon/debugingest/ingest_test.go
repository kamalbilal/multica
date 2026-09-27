package debugingest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIngestCORSAndTextPlain(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.Register("tok", "sess-1")

	opt := httptest.NewRequest(http.MethodOptions, s.IngestURL("tok"), nil)
	opt.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	s.httpSrv.Handler.ServeHTTP(rec, opt)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("CORS origin = %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("must not send credentials CORS")
	}

	body := `{"hypothesis_id":"H1","message":"hit","ts":1,"captured_at":"2026-09-26T00:00:00Z","data":{"k":1},"meta":{"url":"http://localhost:3000/x"}}`
	post := httptest.NewRequest(http.MethodPost, s.IngestURL("tok"), strings.NewReader(body))
	post.Header.Set("Origin", "http://127.0.0.1:5173")
	post.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.httpSrv.Handler.ServeHTTP(rec, post)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d body %s", rec.Code, rec.Body.String())
	}
	dump, err := ReadEvents(root, "sess-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dump, `"hypothesis_id":"H1"`) {
		t.Fatalf("dump = %q", dump)
	}
}

func TestHoldFile(t *testing.T) {
	root := t.TempDir()
	if HasHold(root, "s1") {
		t.Fatal("hold before write")
	}
	if err := WriteHold(root, "s1", "issue-1"); err != nil {
		t.Fatal(err)
	}
	if !HasHold(root, "s1") {
		t.Fatal("hold missing")
	}
	if got := ReadHoldIssueID(root, "s1"); got != "issue-1" {
		t.Fatalf("hold issue id = %q", got)
	}
	if err := WriteHoldMeta(root, "s1", "issue-1", "tok-restore"); err != nil {
		t.Fatal(err)
	}
	if got := ReadHoldToken(root, "s1"); got != "tok-restore" {
		t.Fatalf("hold token = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "debug-sessions", "s1", "hold")); err != nil {
		t.Fatal(err)
	}
	if got := ListHeldSessionIDs(root); len(got) != 1 || got[0] != "s1" {
		t.Fatalf("held sessions = %v", got)
	}
	ClearHold(root, "s1")
	if HasHold(root, "s1") {
		t.Fatal("hold after clear")
	}
}

func TestUnknownTokenRejected(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	post := httptest.NewRequest(http.MethodPost, s.IngestURL("nope"), strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.httpSrv.Handler.ServeHTTP(rec, post)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestIngestSerializesParallelWrites(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.Register("tok", "sess-1")

	const n = 20
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			body := fmt.Sprintf(`{"i":%d}`, i)
			post := httptest.NewRequest(http.MethodPost, s.IngestURL("tok"), strings.NewReader(body))
			post.Header.Set("Content-Type", "text/plain")
			rec := httptest.NewRecorder()
			s.httpSrv.Handler.ServeHTTP(rec, post)
			if rec.Code != http.StatusNoContent {
				errCh <- fmt.Errorf("status = %d", rec.Code)
				return
			}
			errCh <- nil
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	dump, err := ReadEvents(root, "sess-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(dump, "\n"); got != n {
		t.Fatalf("lines = %d dump=%q", got, dump)
	}
}

func TestIngestReportsEventCount(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	got := make(chan struct {
		issueID    string
		sessionID  string
		eventCount int
	}, 1)
	s.OnEvent = func(issueID, sessionID string, eventCount int) {
		got <- struct {
			issueID    string
			sessionID  string
			eventCount int
		}{issueID, sessionID, eventCount}
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.Register("tok", "sess-1")
	if err := WriteHold(root, "sess-1", "issue-9"); err != nil {
		t.Fatal(err)
	}

	post := httptest.NewRequest(http.MethodPost, s.IngestURL("tok"), strings.NewReader(`{"hypothesis_id":"H1"}`))
	post.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	s.httpSrv.Handler.ServeHTTP(rec, post)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}

	select {
	case ev := <-got:
		if ev.issueID != "issue-9" || ev.sessionID != "sess-1" || ev.eventCount != 1 {
			t.Fatalf("OnEvent = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnEvent not called")
	}
}

func TestSyncHeldEventsReportsFileAppend(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	got := make(chan int, 1)
	s.OnEvent = func(issueID, sessionID string, eventCount int) {
		if issueID != "issue-1" || sessionID != "sess-1" {
			t.Errorf("OnEvent ids = %s %s", issueID, sessionID)
		}
		got <- eventCount
	}
	if err := WriteHold(root, "sess-1", "issue-1"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(SessionDir(root, "sess-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(EventsPath(root, "sess-1"), []byte("{\"hypothesis_id\":\"H1\"}\n{\"hypothesis_id\":\"H2\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.SyncHeldEvents()
	select {
	case n := <-got:
		if n != 2 {
			t.Fatalf("eventCount = %d", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnEvent not called for file-appended probes")
	}
	s.SyncHeldEvents()
	select {
	case n := <-got:
		t.Fatalf("duplicate report %d", n)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStartRestoresIngestTokenFromHold(t *testing.T) {
	root := t.TempDir()
	if err := WriteHoldMeta(root, "sess-1", "issue-1", "tok-restore"); err != nil {
		t.Fatal(err)
	}
	s := New(root)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	post := httptest.NewRequest(http.MethodPost, s.IngestURL("tok-restore"), strings.NewReader(`{"hypothesis_id":"H1"}`))
	post.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	s.httpSrv.Handler.ServeHTTP(rec, post)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
}
