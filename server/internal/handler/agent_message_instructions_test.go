package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestAgentMessageInstructionsRoundTrip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}

	var created AgentResponse
	testutil.Call(t, testHandler.CreateAgent, newRequest(http.MethodPost, "/api/agents", map[string]any{
		"name":                 fmt.Sprintf("message-instructions-%d", time.Now().UnixNano()),
		"runtime_id":           handlerTestRuntimeID(t),
		"message_instructions": "Always reply in bullets.",
	})).Want(http.StatusCreated).JSON(&created)
	dbfx.Cleanup(t, `DELETE FROM agent WHERE id = $1`, created.ID)
	if created.MessageInstructions != "Always reply in bullets." {
		t.Fatalf("created message_instructions = %q", created.MessageInstructions)
	}

	var got AgentResponse
	testutil.Call(t, testHandler.GetAgent, withURLParam(
		newRequest(http.MethodGet, "/api/agents/"+created.ID, nil),
		"id",
		created.ID,
	)).Want(http.StatusOK).JSON(&got)
	if got.MessageInstructions != "Always reply in bullets." {
		t.Fatalf("get message_instructions = %q", got.MessageInstructions)
	}

	var preserved AgentResponse
	testutil.Call(t, testHandler.UpdateAgent, withURLParam(
		newRequest(http.MethodPut, "/api/agents/"+created.ID, map[string]any{
			"description": "message instructions unchanged",
		}),
		"id",
		created.ID,
	)).Want(http.StatusOK).JSON(&preserved)
	if preserved.MessageInstructions != "Always reply in bullets." {
		t.Fatalf("omitted update message_instructions = %q, want preserved", preserved.MessageInstructions)
	}

	var cleared AgentResponse
	testutil.Call(t, testHandler.UpdateAgent, withURLParam(
		newRequest(http.MethodPut, "/api/agents/"+created.ID, map[string]any{
			"message_instructions": "",
		}),
		"id",
		created.ID,
	)).Want(http.StatusOK).JSON(&cleared)
	if cleared.MessageInstructions != "" {
		t.Fatalf("cleared message_instructions = %q, want empty", cleared.MessageInstructions)
	}
}

func TestCreateAgent_MessageInstructionsDefaultEmpty(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}

	var created AgentResponse
	testutil.Call(t, testHandler.CreateAgent, newRequest(http.MethodPost, "/api/agents", map[string]any{
		"name":       fmt.Sprintf("message-instructions-empty-%d", time.Now().UnixNano()),
		"runtime_id": handlerTestRuntimeID(t),
	})).Want(http.StatusCreated).JSON(&created)
	dbfx.Cleanup(t, `DELETE FROM agent WHERE id = $1`, created.ID)
	if created.MessageInstructions != "" {
		t.Fatalf("default message_instructions = %q, want empty", created.MessageInstructions)
	}
}

func TestCreateAgent_MessageInstructionsCap(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}

	testutil.Call(t, testHandler.CreateAgent, newRequest(http.MethodPost, "/api/agents", map[string]any{
		"name":                 fmt.Sprintf("message-instructions-cap-%d", time.Now().UnixNano()),
		"runtime_id":           handlerTestRuntimeID(t),
		"message_instructions": strings.Repeat("a", maxAgentMessageInstructionsLength+1),
	})).Want(http.StatusBadRequest)

	var created AgentResponse
	testutil.Call(t, testHandler.CreateAgent, newRequest(http.MethodPost, "/api/agents", map[string]any{
		"name":                 fmt.Sprintf("message-instructions-ok-%d", time.Now().UnixNano()),
		"runtime_id":           handlerTestRuntimeID(t),
		"message_instructions": strings.Repeat("a", maxAgentMessageInstructionsLength),
	})).Want(http.StatusCreated).JSON(&created)
	dbfx.Cleanup(t, `DELETE FROM agent WHERE id = $1`, created.ID)
	if len([]rune(created.MessageInstructions)) != maxAgentMessageInstructionsLength {
		t.Fatalf("at-limit create length = %d", len([]rune(created.MessageInstructions)))
	}

	testutil.Call(t, testHandler.UpdateAgent, withURLParam(
		newRequest(http.MethodPut, "/api/agents/"+created.ID, map[string]any{
			"message_instructions": strings.Repeat("b", maxAgentMessageInstructionsLength+1),
		}),
		"id",
		created.ID,
	)).Want(http.StatusBadRequest)
}
