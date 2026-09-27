package debugingest

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxEventBytes = 64 << 10

// How often file-appended probe lines (MULTICA_DEBUG_LOG_PATH) are reported
// while the human retests. HTTP ingest still reports immediately.
const heldEventPollInterval = 2 * time.Second

// Server is a loopback-only HTTP ingest for debug probe events.
type Server struct {
	mu        sync.Mutex
	ln        net.Listener
	httpSrv   *http.Server
	baseURL   string
	root      string
	sessions  map[string]string // ingest token -> session id
	lastCount map[string]int    // session id -> last reported event count
	stop      chan struct{}
	OnEvent   func(issueID, sessionID string, eventCount int)
}

// Event is one NDJSON probe line. Extra JSON keys are preserved on disk as the raw body.
type Event struct {
	HypothesisID string          `json:"hypothesis_id"`
	Location     string          `json:"location"`
	Message      string          `json:"message"`
	Data         json.RawMessage `json:"data"`
	TS           int64           `json:"ts"`
	CapturedAt   string          `json:"captured_at"`
	RunID        string          `json:"run_id"`
	Meta         json.RawMessage `json:"meta"`
}

func New(root string) *Server {
	return &Server{
		root:      root,
		sessions:  make(map[string]string),
		lastCount: make(map[string]int),
	}
}

func (s *Server) Register(token, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]string)
	}
	s.sessions[token] = sessionID
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest/", s.handleIngest)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	s.ln = ln
	s.httpSrv = srv
	s.baseURL = "http://" + ln.Addr().String()
	s.stop = make(chan struct{})
	s.restoreHeldTokensLocked()
	go func() { _ = srv.Serve(ln) }()
	go s.pollHeldEvents(s.stop)
	return nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	if s.stop != nil {
		select {
		case <-s.stop:
		default:
			close(s.stop)
		}
	}
	if s.httpSrv == nil {
		s.mu.Unlock()
		return nil
	}
	err := s.httpSrv.Close()
	s.ln = nil
	s.httpSrv = nil
	s.baseURL = ""
	s.mu.Unlock()
	return err
}

func (s *Server) BaseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseURL
}

func (s *Server) IngestURL(token string) string {
	return strings.TrimRight(s.BaseURL(), "/") + "/ingest/" + url.PathEscape(token)
}

func SessionDir(root, sessionID string) string {
	return filepath.Join(root, "debug-sessions", sessionID)
}

func EventsPath(root, sessionID string) string {
	return filepath.Join(SessionDir(root, sessionID), "events.jsonl")
}

func HoldPath(root, sessionID string) string {
	return filepath.Join(SessionDir(root, sessionID), "hold")
}

type holdFile struct {
	IssueID     string `json:"issue_id"`
	IngestToken string `json:"ingest_token,omitempty"`
}

func WriteHold(root, sessionID, issueID string) error {
	return WriteHoldMeta(root, sessionID, issueID, "")
}

func WriteHoldMeta(root, sessionID, issueID, ingestToken string) error {
	dir := SessionDir(root, sessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.Marshal(holdFile{IssueID: issueID, IngestToken: ingestToken})
	if err != nil {
		return err
	}
	return os.WriteFile(HoldPath(root, sessionID), append(payload, '\n'), 0o600)
}

func readHold(root, sessionID string) holdFile {
	data, err := os.ReadFile(HoldPath(root, sessionID))
	if err != nil {
		return holdFile{}
	}
	var hold holdFile
	if json.Unmarshal(data, &hold) != nil {
		return holdFile{}
	}
	return hold
}

func ReadHoldIssueID(root, sessionID string) string {
	return strings.TrimSpace(readHold(root, sessionID).IssueID)
}

func ReadHoldToken(root, sessionID string) string {
	return strings.TrimSpace(readHold(root, sessionID).IngestToken)
}

func ListHeldSessionIDs(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, "debug-sessions"))
	if err != nil {
		return nil
	}
	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if HasHold(root, entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	return ids
}

func HasHold(root, sessionID string) bool {
	_, err := os.Stat(HoldPath(root, sessionID))
	return err == nil
}

func ClearHold(root, sessionID string) {
	_ = os.Remove(HoldPath(root, sessionID))
}

func ReadEvents(root, sessionID string, maxBytes int) (string, error) {
	data, err := os.ReadFile(EventsPath(root, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if maxBytes > 0 && len(data) > maxBytes {
		data = data[len(data)-maxBytes:]
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return string(data), nil
}

func CountEventLines(dump string) int {
	n := 0
	for _, line := range strings.Split(dump, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func (s *Server) restoreHeldTokensLocked() {
	if s.sessions == nil {
		s.sessions = make(map[string]string)
	}
	for _, sessionID := range ListHeldSessionIDs(s.root) {
		token := ReadHoldToken(s.root, sessionID)
		if token != "" {
			s.sessions[token] = sessionID
		}
	}
}

func (s *Server) pollHeldEvents(stop <-chan struct{}) {
	ticker := time.NewTicker(heldEventPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.SyncHeldEvents()
		}
	}
}

// SyncHeldEvents reports newly appended probe lines for every held session.
// Safe to call from tests; the wait-loop poller uses it too.
func (s *Server) SyncHeldEvents() {
	for _, sessionID := range ListHeldSessionIDs(s.root) {
		s.reportCount(sessionID)
	}
}

func (s *Server) reportCount(sessionID string) {
	dump, err := ReadEvents(s.root, sessionID, 0)
	if err != nil {
		return
	}
	count := CountEventLines(dump)
	issueID := ReadHoldIssueID(s.root, sessionID)
	s.mu.Lock()
	prev := 0
	if s.lastCount != nil {
		prev = s.lastCount[sessionID]
	} else {
		s.lastCount = make(map[string]int)
	}
	if count <= prev {
		s.mu.Unlock()
		return
	}
	s.lastCount[sessionID] = count
	cb := s.OnEvent
	s.mu.Unlock()
	if cb != nil && issueID != "" && count > 0 {
		go cb(issueID, sessionID, count)
	}
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	applyCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/ingest/")
	token = strings.Trim(token, "/")
	if token == "" || strings.Contains(token, "/") || strings.Contains(token, "..") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxEventBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxEventBytes {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	sessionID := s.sessions[token]
	if sessionID == "" {
		s.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	err = appendEvent(s.root, sessionID, body)
	s.mu.Unlock()
	if err != nil {
		http.Error(w, "write failed", http.StatusInternalServerError)
		return
	}
	dump, _ := ReadEvents(s.root, sessionID, 0)
	count := CountEventLines(dump)
	s.mu.Lock()
	if s.lastCount == nil {
		s.lastCount = make(map[string]int)
	}
	s.lastCount[sessionID] = count
	cb := s.OnEvent
	s.mu.Unlock()
	issueID := ReadHoldIssueID(s.root, sessionID)
	if cb != nil && issueID != "" && count > 0 {
		go cb(issueID, sessionID, count)
	}
	w.WriteHeader(http.StatusNoContent)
}

func appendEvent(root, sessionID string, body []byte) error {
	dir := SessionDir(root, sessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	line := bytesTrimSpace(body)
	f, err := os.OpenFile(EventsPath(root, sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func applyCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allow := "*"
	if isLoopbackOrigin(origin) {
		allow = origin
	}
	w.Header().Set("Access-Control-Allow-Origin", allow)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Vary", "Origin")
}

func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
