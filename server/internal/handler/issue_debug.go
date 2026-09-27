package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/dbid"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const (
	debugStatusInstrumenting = "instrumenting"
	debugStatusWaitingRepro  = "waiting_repro"
	debugStatusAnalyzing     = "analyzing"
	debugStatusWaitingVerify = "waiting_verify"
	debugStatusClosed        = "closed"

	debugActionReproduced = "reproduced"
	debugActionComment    = "comment"
	debugActionFixed      = "fixed"

	debugLogDumpMaxBytes = 256 << 10
)

type IssueDebugSessionResponse struct {
	ID                string          `json:"id"`
	IssueID           string          `json:"issue_id"`
	AgentID           string          `json:"agent_id"`
	SourceTaskID      string          `json:"source_task_id,omitempty"`
	WaitCommentID     string          `json:"wait_comment_id,omitempty"`
	Status            string          `json:"status"`
	Hypotheses        json.RawMessage `json:"hypotheses"`
	ReproSteps        string          `json:"repro_steps"`
	ProbePaths        json.RawMessage `json:"probe_paths"`
	ContinueAction    string          `json:"continue_action,omitempty"`
	ContinueCommentID string          `json:"continue_comment_id,omitempty"`
	WorkDirHint       string          `json:"work_dir_hint,omitempty"`
	EventCount        int             `json:"event_count"`
	Revision          int64           `json:"revision"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

type issueDebugSessionEnvelope struct {
	Session *IssueDebugSessionResponse `json:"session"`
}

type issueDebugWaitRequest struct {
	Hypotheses    json.RawMessage `json:"hypotheses"`
	ReproSteps    string          `json:"repro_steps"`
	ProbePaths    json.RawMessage `json:"probe_paths"`
	WaitCommentID string          `json:"wait_comment_id"`
	WorkDirHint   string          `json:"work_dir_hint"`
	Status        string          `json:"status"`
}

type issueDebugContinueRequest struct {
	Action    string `json:"action"`
	CommentID string `json:"comment_id,omitempty"`
	Content   string `json:"content,omitempty"`
	ParentID  string `json:"parent_id,omitempty"`
}

func debugSessionToResponse(row db.IssueDebugSession) IssueDebugSessionResponse {
	hyp := json.RawMessage(row.Hypotheses)
	if len(hyp) == 0 {
		hyp = json.RawMessage("[]")
	}
	paths := json.RawMessage(row.ProbePaths)
	if len(paths) == 0 {
		paths = json.RawMessage("[]")
	}
	return IssueDebugSessionResponse{
		ID:                uuidToString(row.ID),
		IssueID:           uuidToString(row.IssueID),
		AgentID:           uuidToString(row.AgentID),
		SourceTaskID:      optionalUUIDString(row.SourceTaskID),
		WaitCommentID:     optionalUUIDString(row.WaitCommentID),
		Status:            row.Status,
		Hypotheses:        hyp,
		ReproSteps:        row.ReproSteps,
		ProbePaths:        paths,
		ContinueAction:    row.ContinueAction,
		ContinueCommentID: optionalUUIDString(row.ContinueCommentID),
		WorkDirHint:       row.WorkDirHint,
		EventCount:        int(row.EventCount),
		Revision:          row.Revision,
		CreatedAt:         timestampToString(row.CreatedAt),
		UpdatedAt:         timestampToString(row.UpdatedAt),
	}
}

func optionalUUIDString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuidToString(u)
}

func newIngestToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func debugSessionIsWaiting(status string) bool {
	return status == debugStatusWaitingRepro || status == debugStatusWaitingVerify
}

func (h *Handler) listOpenDebugSessions(ctx context.Context, issue db.Issue) ([]db.IssueDebugSession, error) {
	return h.Queries.ListOpenIssueDebugSessions(ctx, db.ListOpenIssueDebugSessionsParams{
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
	})
}

func waitingDebugSession(rows []db.IssueDebugSession) (db.IssueDebugSession, bool) {
	for _, row := range rows {
		if debugSessionIsWaiting(row.Status) {
			return row, true
		}
	}
	return db.IssueDebugSession{}, false
}

func preferredOpenDebugSession(rows []db.IssueDebugSession) (db.IssueDebugSession, bool) {
	if row, ok := waitingDebugSession(rows); ok {
		return row, true
	}
	if len(rows) == 0 {
		return db.IssueDebugSession{}, false
	}
	return rows[0], true
}

func (h *Handler) GetIssueDebugSession(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	rows, err := h.listOpenDebugSessions(r.Context(), issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load debug session")
		return
	}
	row, found := preferredOpenDebugSession(rows)
	if !found {
		writeJSON(w, http.StatusOK, issueDebugSessionEnvelope{})
		return
	}
	resp := debugSessionToResponse(row)
	writeJSON(w, http.StatusOK, issueDebugSessionEnvelope{Session: &resp})
}

func (h *Handler) WaitIssueDebugSession(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	if actorType != "agent" {
		writeError(w, http.StatusForbidden, "only the running agent can wait a debug session")
		return
	}
	agentUUID, ok := parseUUIDOrBadRequest(w, actorID, "agent id")
	if !ok {
		return
	}
	session, err := h.Queries.GetOpenIssueDebugSession(r.Context(), db.GetOpenIssueDebugSessionParams{
		IssueID:     issue.ID,
		AgentID:     agentUUID,
		WorkspaceID: issue.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "no open debug session")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load debug session")
		return
	}
	var req issueDebugWaitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := req.Status
	if status == "" {
		status = debugStatusWaitingRepro
	}
	if status != debugStatusWaitingRepro && status != debugStatusWaitingVerify {
		writeError(w, http.StatusBadRequest, "invalid wait status")
		return
	}
	hyp := req.Hypotheses
	if len(hyp) == 0 {
		hyp = json.RawMessage("[]")
	}
	paths := req.ProbePaths
	if len(paths) == 0 {
		paths = json.RawMessage("[]")
	}
	var waitComment pgtype.UUID
	if req.WaitCommentID != "" {
		waitComment, ok = parseUUIDOrBadRequest(w, req.WaitCommentID, "wait_comment_id")
		if !ok {
			return
		}
	}
	updated, err := h.Queries.UpdateIssueDebugSessionWait(r.Context(), db.UpdateIssueDebugSessionWaitParams{
		ID:            session.ID,
		WorkspaceID:   issue.WorkspaceID,
		Status:        status,
		Hypotheses:    hyp,
		ReproSteps:    req.ReproSteps,
		ProbePaths:    paths,
		WaitCommentID: waitComment,
		WorkDirHint:   req.WorkDirHint,
		SourceTaskID:  h.sourceTaskIDFromRequest(r),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update debug session")
		return
	}
	resp := debugSessionToResponse(updated)
	h.publishDebugSession(issue, actorType, actorID, resp)
	writeJSON(w, http.StatusOK, issueDebugSessionEnvelope{Session: &resp})
}

func (h *Handler) ContinueIssueDebugSession(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	if actorType != "member" {
		writeError(w, http.StatusForbidden, "only a workspace member can continue a debug session")
		return
	}
	rows, err := h.listOpenDebugSessions(r.Context(), issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load debug session")
		return
	}
	session, found := waitingDebugSession(rows)
	if !found {
		writeError(w, http.StatusNotFound, "no waiting debug session")
		return
	}
	var req issueDebugContinueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	action := strings.TrimSpace(req.Action)
	if action != debugActionReproduced && action != debugActionComment && action != debugActionFixed {
		writeError(w, http.StatusBadRequest, "invalid continue action")
		return
	}
	content := strings.TrimSpace(req.Content)
	switch action {
	case debugActionReproduced:
		if content == "" {
			content = "Approved"
		}
	case debugActionFixed:
		if content == "" {
			content = "Looks fixed"
		}
	default:
		if content == "" {
			writeError(w, http.StatusBadRequest, "content is required")
			return
		}
	}
	parentID := session.WaitCommentID
	if req.ParentID != "" {
		parentID, ok = parseUUIDOrBadRequest(w, req.ParentID, "parent_id")
		if !ok {
			return
		}
	}
	agent, err := h.Queries.GetAgent(r.Context(), session.AgentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load agent")
		return
	}
	mention := "[@" + agent.Name + "](mention://agent/" + uuidToString(agent.ID) + ")"
	body := mention + " " + content
	comment, err := h.Queries.CreateComment(r.Context(), db.CreateCommentParams{
		ID:          dbid.NewV7(),
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
		AuthorType:  "member",
		AuthorID:    parseUUID(actorID),
		Content:     body,
		Type:        "comment",
		ParentID:    parentID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to post continue comment")
		return
	}
	dbComment := comment.Comment()
	updated, err := h.Queries.UpdateIssueDebugSessionContinue(r.Context(), db.UpdateIssueDebugSessionContinueParams{
		ID:                session.ID,
		WorkspaceID:       issue.WorkspaceID,
		Status:            debugStatusAnalyzing,
		ContinueAction:    action,
		ContinueCommentID: dbComment.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to continue debug session")
		return
	}
	groupedAtt := h.groupAttachments(r, []pgtype.UUID{dbComment.ID})
	commentResp := commentToResponse(dbComment, nil, groupedAtt[uuidToString(dbComment.ID)])
	h.publish(protocol.EventCommentCreated, uuidToString(issue.WorkspaceID), "member", actorID, map[string]any{
		"comment": commentResp,
	})
	h.triggerTasksForComment(r.Context(), issue, dbComment, nil, "member", actorID, actorID, nil, nil)
	resp := debugSessionToResponse(updated)
	h.publishDebugSession(issue, "member", actorID, resp)
	writeJSON(w, http.StatusOK, issueDebugSessionEnvelope{Session: &resp})
}

func (h *Handler) ensureDebugSessionsForComment(ctx context.Context, issue db.Issue, enqueued map[string]commentEnqueueResult, debugMode bool) {
	if !debugMode {
		return
	}
	existing, err := h.listOpenDebugSessions(ctx, issue)
	if err != nil {
		slog.Error("debug session: list open failed", "error", err)
		return
	}
	if _, waiting := waitingDebugSession(existing); waiting {
		return
	}
	for agentID, result := range enqueued {
		if result.status != DispatchQueued && result.status != DispatchCoalesced && result.status != DispatchDeferred {
			continue
		}
		parsed, err := util.ParseUUID(agentID)
		if err != nil {
			continue
		}
		agent, err := h.Queries.GetAgent(ctx, parsed)
		if err != nil {
			slog.Error("debug session: load agent failed", "agent_id", agentID, "error", err)
			continue
		}
		if !h.agentRuntimeHasDebugIngest(ctx, agent) {
			slog.Info("debug session: skipped, runtime lacks debug-ingest-v1", "agent_id", agentID)
			continue
		}
		_, err = h.Queries.GetOpenIssueDebugSession(ctx, db.GetOpenIssueDebugSessionParams{
			IssueID:     issue.ID,
			AgentID:     parsed,
			WorkspaceID: issue.WorkspaceID,
		})
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("debug session: lookup failed", "agent_id", agentID, "error", err)
			continue
		}
		token, tokErr := newIngestToken()
		if tokErr != nil {
			slog.Error("debug session: ingest token failed", "error", tokErr)
			continue
		}
		if _, err = h.Queries.CreateIssueDebugSession(ctx, db.CreateIssueDebugSessionParams{
			ID:          dbid.NewV7(),
			WorkspaceID: issue.WorkspaceID,
			IssueID:     issue.ID,
			AgentID:     parsed,
			IngestToken: token,
		}); err != nil {
			slog.Error("debug session: create failed", "agent_id", agentID, "error", err)
		}
	}
}

func (h *Handler) agentRuntimeHasDebugIngest(ctx context.Context, agent db.Agent) bool {
	if !agent.RuntimeID.Valid {
		return false
	}
	runtime, err := h.getAgentRuntime(ctx, obsmetrics.RuntimeLookupSourceComment, agent.RuntimeID)
	if err != nil {
		return false
	}
	return runtimeHasCapability(runtime.Metadata, protocol.DaemonCapabilityDebugIngestV1)
}

func (h *Handler) rejectDebugModeIfUnavailable(w http.ResponseWriter, r *http.Request, issue db.Issue, content string, parentComment *db.Comment, authorType, authorID string, suppressAgentIDs []pgtype.UUID) bool {
	originator := h.invokeOriginatorFromRequest(r, authorType, authorID)
	triggers, _ := h.computeCommentAgentTriggers(r.Context(), issue, content, parentComment, authorType, authorID, commentTriggerComputeOptions{
		OriginatorUserID: originator,
	})
	triggers = filterSuppressedCommentAgentTriggers(triggers, suppressAgentIDs)
	if len(triggers) == 0 {
		writeError(w, http.StatusBadRequest, "mention or assign an agent before starting Debug")
		return false
	}
	open, err := h.listOpenDebugSessions(r.Context(), issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load debug session")
		return false
	}
	if _, waiting := waitingDebugSession(open); waiting {
		writeError(w, http.StatusConflict, "a debug session is already waiting on this issue")
		return false
	}
	capable := 0
	for _, trigger := range triggers {
		if h.agentRuntimeHasDebugIngest(r.Context(), trigger.Agent) {
			capable++
		}
	}
	if capable == 0 {
		writeError(w, http.StatusUnprocessableEntity, "the agent's desktop daemon needs an update before Debug can start")
		return false
	}
	return true
}

func (h *Handler) PutDaemonDebugSessionLogs(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueId")
	issueUUID, ok := parseUUIDOrBadRequest(w, issueID, "issue_id")
	if !ok {
		return
	}
	issue, err := h.Queries.GetIssue(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if !h.requireDaemonWorkspaceAccess(w, r, uuidToString(issue.WorkspaceID)) {
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		LogDump   string `json:"log_dump"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.SessionID) == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sessionUUID, ok := parseUUIDOrBadRequest(w, req.SessionID, "session_id")
	if !ok {
		return
	}
	session, err := h.Queries.GetIssueDebugSession(r.Context(), db.GetIssueDebugSessionParams{
		ID:          sessionUUID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil || uuidToString(session.IssueID) != uuidToString(issue.ID) {
		writeError(w, http.StatusNotFound, "debug session not found")
		return
	}
	dump := req.LogDump
	if len(dump) > debugLogDumpMaxBytes {
		dump = dump[len(dump)-debugLogDumpMaxBytes:]
	}
	updated, err := h.Queries.UpdateIssueDebugSessionLogDump(r.Context(), db.UpdateIssueDebugSessionLogDumpParams{
		ID:          session.ID,
		WorkspaceID: issue.WorkspaceID,
		LogDump:     dump,
		EventCount:  int32(debugLogEventCount(dump)),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store debug logs")
		return
	}
	h.publishDebugSession(issue, "agent", uuidToString(updated.AgentID), debugSessionToResponse(updated))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PutDaemonDebugSessionEvents(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueId")
	issueUUID, ok := parseUUIDOrBadRequest(w, issueID, "issue_id")
	if !ok {
		return
	}
	issue, err := h.Queries.GetIssue(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if !h.requireDaemonWorkspaceAccess(w, r, uuidToString(issue.WorkspaceID)) {
		return
	}
	var req struct {
		SessionID  string `json:"session_id"`
		EventCount int    `json:"event_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.SessionID) == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.EventCount < 0 {
		req.EventCount = 0
	}
	sessionUUID, ok := parseUUIDOrBadRequest(w, req.SessionID, "session_id")
	if !ok {
		return
	}
	updated, err := h.Queries.UpdateIssueDebugSessionEventCount(r.Context(), db.UpdateIssueDebugSessionEventCountParams{
		ID:          sessionUUID,
		WorkspaceID: issue.WorkspaceID,
		EventCount:  int32(req.EventCount),
	})
	if err != nil || uuidToString(updated.IssueID) != uuidToString(issue.ID) {
		writeError(w, http.StatusNotFound, "debug session not found")
		return
	}
	h.publishDebugSession(issue, "agent", uuidToString(updated.AgentID), debugSessionToResponse(updated))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetDaemonDebugSession(w http.ResponseWriter, r *http.Request) {
	issueID := chi.URLParam(r, "issueId")
	sessionID := chi.URLParam(r, "sessionId")
	issueUUID, ok := parseUUIDOrBadRequest(w, issueID, "issue_id")
	if !ok {
		return
	}
	sessionUUID, ok := parseUUIDOrBadRequest(w, sessionID, "session_id")
	if !ok {
		return
	}
	issue, err := h.Queries.GetIssue(r.Context(), issueUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if !h.requireDaemonWorkspaceAccess(w, r, uuidToString(issue.WorkspaceID)) {
		return
	}
	session, err := h.Queries.GetIssueDebugSession(r.Context(), db.GetIssueDebugSessionParams{
		ID:          sessionUUID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil || uuidToString(session.IssueID) != uuidToString(issue.ID) {
		writeError(w, http.StatusNotFound, "debug session not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":     uuidToString(session.ID),
		"status": session.Status,
	})
}

func (h *Handler) CloseIssueDebugSession(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	rows, err := h.listOpenDebugSessions(r.Context(), issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load debug session")
		return
	}
	session, found := preferredOpenDebugSession(rows)
	if !found {
		writeError(w, http.StatusNotFound, "no open debug session")
		return
	}
	if actorType == "agent" && uuidToString(session.AgentID) != actorID {
		writeError(w, http.StatusForbidden, "only the debug session agent can close it")
		return
	}
	if actorType != "agent" && actorType != "member" {
		writeError(w, http.StatusForbidden, "not allowed")
		return
	}
	closed, err := h.Queries.CloseIssueDebugSession(r.Context(), db.CloseIssueDebugSessionParams{
		ID:          session.ID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to close debug session")
		return
	}
	resp := debugSessionToResponse(closed)
	h.publishDebugSession(issue, actorType, actorID, resp)
	writeJSON(w, http.StatusOK, issueDebugSessionEnvelope{Session: &resp})
}

func (h *Handler) publishDebugSession(issue db.Issue, actorType, actorID string, session IssueDebugSessionResponse) {
	h.publish(protocol.EventDebugSessionUpdated, uuidToString(issue.WorkspaceID), actorType, actorID, map[string]any{
		"issue_id": uuidToString(issue.ID),
		"session":  session,
	})
}

func (h *Handler) sourceTaskIDFromRequest(r *http.Request) pgtype.UUID {
	if task, ok := h.taskFromRequestHeader(r); ok {
		return task.ID
	}
	return pgtype.UUID{}
}

func debugLogEventCount(dump string) int {
	n := 0
	for _, line := range strings.Split(dump, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func debugSessionForClaim(row db.IssueDebugSession) *TaskDebugSessionData {
	dump := row.LogDump
	if len(dump) > debugLogDumpMaxBytes {
		dump = dump[len(dump)-debugLogDumpMaxBytes:]
	}
	hyp := json.RawMessage(row.Hypotheses)
	if len(hyp) == 0 {
		hyp = json.RawMessage("[]")
	}
	return &TaskDebugSessionData{
		ID:             uuidToString(row.ID),
		Status:         row.Status,
		ContinueAction: row.ContinueAction,
		Hypotheses:     hyp,
		ReproSteps:     row.ReproSteps,
		LogDump:        dump,
		IngestToken:    row.IngestToken,
		WaitCommentID:  optionalUUIDString(row.WaitCommentID),
	}
}
