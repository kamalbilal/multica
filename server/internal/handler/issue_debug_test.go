package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestIssueDebugSessionGetWaitAndContinue(t *testing.T) {
	runtimeID := dbfx.Runtime(t, "debug session runtime")
	agentID := dbfx.Agent(t, "debug session agent", runtimeID)
	issueID := dbfx.Issue(t, "debug session issue", testutil.Cols{"assignee_type": "agent", "assignee_id": agentID})
	dbfx.Cleanup(t, "DELETE FROM issue_debug_session WHERE issue_id = $1", issueID)

	empty := testutil.Call(t, testHandler.GetIssueDebugSession, withURLParam(
		newRequest("GET", "/api/issues/"+issueID+"/debug", nil), "id", issueID,
	)).Want(http.StatusOK)
	var envelope issueDebugSessionEnvelope
	empty.JSON(&envelope)
	if envelope.Session != nil {
		t.Fatal("expected no open session")
	}

	sessionID := dbfx.Insert(t, "issue_debug_session", testutil.Cols{
		"workspace_id": testWorkspaceID,
		"issue_id":     issueID,
		"agent_id":     agentID,
		"ingest_token": "ingest-token",
		"status":       "waiting_repro",
		"repro_steps":  "open the page",
	})

	got := testutil.Call(t, testHandler.GetIssueDebugSession, withURLParam(
		newRequest("GET", "/api/issues/"+issueID+"/debug", nil), "id", issueID,
	)).Want(http.StatusOK)
	got.JSON(&envelope)
	if envelope.Session == nil || envelope.Session.ID != sessionID || envelope.Session.Status != "waiting_repro" {
		t.Fatalf("GET session = %+v", envelope.Session)
	}

	memberWait := testutil.Call(t, testHandler.WaitIssueDebugSession, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/debug/wait", map[string]any{"repro_steps": "x"}), "id", issueID,
	)).Want(http.StatusForbidden)
	_ = memberWait

	continued := testutil.Call(t, testHandler.ContinueIssueDebugSession, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/debug/continue", map[string]any{"action": "reproduced"}), "id", issueID,
	)).Want(http.StatusOK)
	continued.JSON(&envelope)
	if envelope.Session == nil || envelope.Session.Status != debugStatusAnalyzing || envelope.Session.ContinueAction != debugActionReproduced {
		t.Fatalf("continue session = %+v", envelope.Session)
	}
}

func TestIssueDebugSessionWaitRejectsMember(t *testing.T) {
	runtimeID := dbfx.Runtime(t, "debug wait runtime")
	agentID := dbfx.Agent(t, "debug wait agent", runtimeID)
	issueID := dbfx.Issue(t, "debug wait issue")
	dbfx.Cleanup(t, "DELETE FROM issue_debug_session WHERE issue_id = $1", issueID)
	dbfx.Insert(t, "issue_debug_session", testutil.Cols{
		"workspace_id": testWorkspaceID,
		"issue_id":     issueID,
		"agent_id":     agentID,
		"ingest_token": "tok",
		"status":       "instrumenting",
	})
	testutil.Call(t, testHandler.WaitIssueDebugSession, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/debug/wait", map[string]any{"status": "waiting_repro"}), "id", issueID,
	)).Want(http.StatusForbidden)
}

func seedDebugTestAgent(t *testing.T, name, runtimeID string) string {
	t.Helper()
	agentID := dbfx.Agent(t, name, runtimeID, testutil.Cols{
		"visibility":      "workspace",
		"permission_mode": "public_to",
	})
	dbfx.Exec(t, `
		INSERT INTO agent_invocation_target (agent_id, target_type, target_id)
		VALUES ($1, 'workspace', $2)
		ON CONFLICT (agent_id, target_type, target_id) DO NOTHING
	`, agentID, testWorkspaceID)
	return agentID
}

func TestCreateCommentDebugModeRequiresAgent(t *testing.T) {
	issueID := dbfx.Issue(t, "debug no agent")
	testutil.Call(t, testHandler.CreateComment, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/comments", map[string]any{
			"content":    "plain note",
			"debug_mode": true,
		}), "id", issueID,
	)).Want(http.StatusBadRequest)
}

func TestCreateCommentDebugModeRequiresIngestCapability(t *testing.T) {
	runtimeID := dbfx.Runtime(t, "debug old runtime")
	agentID := seedDebugTestAgent(t, "debug old agent", runtimeID)
	issueID := dbfx.Issue(t, "debug old daemon", testutil.Cols{"assignee_type": "agent", "assignee_id": agentID})
	content := fmt.Sprintf("[@Agent](mention://agent/%s) start debug", agentID)
	testutil.Call(t, testHandler.CreateComment, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/comments", map[string]any{
			"content":    content,
			"debug_mode": true,
		}), "id", issueID,
	)).Want(http.StatusUnprocessableEntity)
}

func TestCreateCommentDebugModeCreatesSessionWhenCapable(t *testing.T) {
	runtimeID := dbfx.Runtime(t, "debug capable runtime", testutil.Cols{
		"metadata": testutil.Raw(fmt.Sprintf(`'{"capabilities":[%q]}'::jsonb`, protocol.DaemonCapabilityDebugIngestV1)),
	})
	agentID := seedDebugTestAgent(t, "debug capable agent", runtimeID)
	issueID := dbfx.Issue(t, "debug capable issue", testutil.Cols{"assignee_type": "agent", "assignee_id": agentID})
	dbfx.Cleanup(t, "DELETE FROM issue_debug_session WHERE issue_id = $1", issueID)
	content := fmt.Sprintf("[@Agent](mention://agent/%s) start debug", agentID)
	created := testutil.Call(t, testHandler.CreateComment, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/comments", map[string]any{
			"content":    content,
			"debug_mode": true,
		}), "id", issueID,
	)).Want(http.StatusCreated)
	_ = created

	got := testutil.Call(t, testHandler.GetIssueDebugSession, withURLParam(
		newRequest("GET", "/api/issues/"+issueID+"/debug", nil), "id", issueID,
	)).Want(http.StatusOK)
	var envelope issueDebugSessionEnvelope
	got.JSON(&envelope)
	if envelope.Session == nil || envelope.Session.AgentID != agentID {
		t.Fatalf("expected open debug session for %s, got %+v", agentID, envelope.Session)
	}
}

func TestContinueDebugSessionEnqueuesFollowUp(t *testing.T) {
	runtimeID := dbfx.Runtime(t, "debug continue runtime", testutil.Cols{
		"metadata": testutil.Raw(fmt.Sprintf(`'{"capabilities":[%q]}'::jsonb`, protocol.DaemonCapabilityDebugIngestV1)),
	})
	agentID := seedDebugTestAgent(t, "debug continue agent", runtimeID)
	issueID := dbfx.Issue(t, "debug continue issue", testutil.Cols{"assignee_type": "agent", "assignee_id": agentID})
	dbfx.Cleanup(t, "DELETE FROM issue_debug_session WHERE issue_id = $1", issueID)
	dbfx.Insert(t, "issue_debug_session", testutil.Cols{
		"workspace_id": testWorkspaceID,
		"issue_id":     issueID,
		"agent_id":     agentID,
		"ingest_token": "ingest-token",
		"status":       "waiting_repro",
		"repro_steps":  "open the page",
	})
	testutil.Call(t, testHandler.ContinueIssueDebugSession, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/debug/continue", map[string]any{"action": "reproduced"}), "id", issueID,
	)).Want(http.StatusOK)
	if got := countQueuedCommentTriggerTasks(t, issueID, agentID); got < 1 {
		t.Fatalf("continue did not enqueue a follow-up task, queued=%d", got)
	}
}

func TestCreateCommentDebugModeRejectsWaitingSession(t *testing.T) {
	runtimeID := dbfx.Runtime(t, "debug waiting runtime", testutil.Cols{
		"metadata": testutil.Raw(fmt.Sprintf(`'{"capabilities":[%q]}'::jsonb`, protocol.DaemonCapabilityDebugIngestV1)),
	})
	agentID := seedDebugTestAgent(t, "debug waiting agent", runtimeID)
	issueID := dbfx.Issue(t, "debug waiting issue", testutil.Cols{"assignee_type": "agent", "assignee_id": agentID})
	dbfx.Cleanup(t, "DELETE FROM issue_debug_session WHERE issue_id = $1", issueID)
	dbfx.Insert(t, "issue_debug_session", testutil.Cols{
		"workspace_id": testWorkspaceID,
		"issue_id":     issueID,
		"agent_id":     agentID,
		"ingest_token": "ingest-token",
		"status":       "waiting_repro",
	})
	content := fmt.Sprintf("[@Agent](mention://agent/%s) start debug", agentID)
	testutil.Call(t, testHandler.CreateComment, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/comments", map[string]any{
			"content":    content,
			"debug_mode": true,
		}), "id", issueID,
	)).Want(http.StatusConflict)
}

func TestCreateCommentDebugModeIgnoresSteerTaskIDs(t *testing.T) {
	runtimeID := dbfx.Runtime(t, "debug no-steer runtime", testutil.Cols{
		"metadata": testutil.Raw(fmt.Sprintf(`'{"capabilities":[%q]}'::jsonb`, protocol.DaemonCapabilityDebugIngestV1)),
	})
	agentID := seedDebugTestAgent(t, "debug no-steer agent", runtimeID)
	issueID := dbfx.Issue(t, "debug no-steer issue", testutil.Cols{"assignee_type": "agent", "assignee_id": agentID})
	dbfx.Cleanup(t, "DELETE FROM issue_debug_session WHERE issue_id = $1", issueID)
	taskID := dbfx.Task(t, agentID, testutil.Cols{
		"issue_id":   issueID,
		"runtime_id": runtimeID,
		"status":     "running",
		"started_at": testutil.Raw("now()"),
	})
	dbfx.Exec(t, `INSERT INTO task_supplement_capability (task_id, workspace_id, issue_id, capability) VALUES ($1, $2, $3, $4)`,
		taskID, testWorkspaceID, issueID, protocol.DaemonCapabilityTaskSupplementV1)
	dbfx.Cleanup(t, `DELETE FROM task_supplement_capability WHERE task_id = $1`, taskID)
	dbfx.Cleanup(t, `DELETE FROM task_supplement WHERE task_id = $1`, taskID)

	content := fmt.Sprintf("[@Agent](mention://agent/%s) start debug", agentID)
	var created CommentResponse
	testutil.Call(t, testHandler.CreateComment, withURLParam(
		newRequest("POST", "/api/issues/"+issueID+"/comments", map[string]any{
			"content":        content,
			"debug_mode":     true,
			"steer_task_ids": []string{taskID},
		}), "id", issueID,
	)).Want(http.StatusCreated).JSON(&created)
	if len(created.Supplements) != 0 {
		t.Fatalf("debug_mode must not steer, supplements=%+v", created.Supplements)
	}

	got := testutil.Call(t, testHandler.GetIssueDebugSession, withURLParam(
		newRequest("GET", "/api/issues/"+issueID+"/debug", nil), "id", issueID,
	)).Want(http.StatusOK)
	var envelope issueDebugSessionEnvelope
	got.JSON(&envelope)
	if envelope.Session == nil || envelope.Session.AgentID != agentID {
		t.Fatalf("expected open debug session, got %+v", envelope.Session)
	}
}

func TestIssueDebugSessionEventCountFromDaemon(t *testing.T) {
	runtimeID := dbfx.Runtime(t, "debug event-count runtime")
	agentID := dbfx.Agent(t, "debug event-count agent", runtimeID)
	issueID := dbfx.Issue(t, "debug event-count issue")
	dbfx.Cleanup(t, "DELETE FROM issue_debug_session WHERE issue_id = $1", issueID)
	sessionID := dbfx.Insert(t, "issue_debug_session", testutil.Cols{
		"workspace_id": testWorkspaceID,
		"issue_id":     issueID,
		"agent_id":     agentID,
		"ingest_token": "ingest-count",
		"status":       "waiting_repro",
	})

	testutil.Call(t, testHandler.PutDaemonDebugSessionEvents, withURLParam(
		newDaemonTokenRequest("POST", "/api/daemon/issues/"+issueID+"/debug-events", map[string]any{
			"session_id":  sessionID,
			"event_count": 4,
		}, testWorkspaceID, "debug-event-count-daemon"),
		"issueId", issueID,
	)).Want(http.StatusNoContent)

	got := testutil.Call(t, testHandler.GetIssueDebugSession, withURLParam(
		newRequest("GET", "/api/issues/"+issueID+"/debug", nil), "id", issueID,
	)).Want(http.StatusOK)
	var envelope issueDebugSessionEnvelope
	got.JSON(&envelope)
	if envelope.Session == nil || envelope.Session.EventCount != 4 {
		t.Fatalf("event_count = %+v", envelope.Session)
	}
}
