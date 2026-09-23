package agent

import (
	"encoding/json"
	"fmt"
)

const (
	cursorSdkCommandExecute    = "execute"
	cursorSdkCommandSteer      = "steer"
	cursorSdkCommandCancel     = "cancel"
	cursorSdkCommandReload     = "reload"
	cursorSdkCommandShutdown   = "shutdown"
	cursorSdkCommandListModels = "list-models"

	cursorSdkEventAgentID = "agent_id"
	cursorSdkEventMessage = "message"
	cursorSdkEventModels  = "models"
	cursorSdkEventError   = "error"
	cursorSdkEventResult  = "result"
)

// CursorSdkExecuteCommand is the Go → Node execute IPC payload.
type CursorSdkExecuteCommand struct {
	Cmd            string         `json:"cmd"`
	ID             string         `json:"id"`
	Prompt         string         `json:"prompt"`
	Cwd            string         `json:"cwd"`
	AgentID        *string        `json:"agentId,omitempty"`
	Model          string         `json:"model"`
	McpConfig      map[string]any `json:"mcpConfig,omitempty"`
	CustomTools    map[string]any `json:"customTools,omitempty"`
	SandboxOptions map[string]any `json:"sandboxOptions,omitempty"`
	APIKeyEnv      string         `json:"apiKeyEnv,omitempty"`
}

// CursorSdkSteerCommand is the Go → Node steer IPC payload.
type CursorSdkSteerCommand struct {
	Cmd  string `json:"cmd"`
	ID   string `json:"id"`
	Text string `json:"text"`
}

// CursorSdkCancelCommand is the Go → Node cancel IPC payload.
type CursorSdkCancelCommand struct {
	Cmd string `json:"cmd"`
	ID  string `json:"id"`
}

// CursorSdkReloadCommand is the Go → Node reload IPC payload.
type CursorSdkReloadCommand struct {
	Cmd string `json:"cmd"`
	ID  string `json:"id"`
}

// CursorSdkShutdownCommand is the Go → Node shutdown IPC payload.
type CursorSdkShutdownCommand struct {
	Cmd string `json:"cmd"`
	ID  string `json:"id"`
}

// CursorSdkListModelsCommand is the Go → Node list-models IPC payload.
type CursorSdkListModelsCommand struct {
	Cmd       string `json:"cmd"`
	ID        string `json:"id"`
	APIKeyEnv string `json:"apiKeyEnv,omitempty"`
}

// CursorSdkExecuteRequest configures a single execute call on the IPC client.
type CursorSdkExecuteRequest struct {
	Prompt         string
	Cwd            string
	AgentID        string
	Model          string
	McpConfig      map[string]any
	CustomTools    map[string]any
	SandboxOptions map[string]any
	APIKeyEnv      string
}

// CursorSdkEvent is a parsed Node → Go IPC event.
type CursorSdkEvent struct {
	Event string

	AgentID string

	MessageType string
	Content     string
	Tool        string
	CallID      string
	Input       json.RawMessage
	Output      string
	Status      string
	Usage       json.RawMessage

	Items []json.RawMessage

	Message   string
	Retryable bool

	ResultStatus string
	ResultOutput string
	ResultUsage  json.RawMessage
	ResultError  string
}

// CursorSdkResult is the terminal execute outcome.
type CursorSdkResult struct {
	Status  string
	Output  string
	Usage   json.RawMessage
	Error   string
	AgentID string
}

func parseCursorSdkEvent(line []byte) (CursorSdkEvent, error) {
	var base struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(line, &base); err != nil {
		return CursorSdkEvent{}, fmt.Errorf("decode cursor sdk event: %w", err)
	}
	if base.Event == "" {
		return CursorSdkEvent{}, fmt.Errorf("decode cursor sdk event: missing event field")
	}

	switch base.Event {
	case cursorSdkEventAgentID:
		var evt CursorSdkAgentIDEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			return CursorSdkEvent{}, fmt.Errorf("decode cursor sdk agent_id event: %w", err)
		}
		return CursorSdkEvent{Event: evt.Event, AgentID: evt.AgentID}, nil
	case cursorSdkEventMessage:
		var evt CursorSdkMessageEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			return CursorSdkEvent{}, fmt.Errorf("decode cursor sdk message event: %w", err)
		}
		return CursorSdkEvent{
			Event:       evt.Event,
			MessageType: evt.Type,
			Content:     evt.Content,
			Tool:        evt.Tool,
			CallID:      evt.CallID,
			Input:       evt.Input,
			Output:      evt.Output,
			Status:      evt.Status,
			Usage:       evt.Usage,
		}, nil
	case cursorSdkEventModels:
		var evt CursorSdkModelsEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			return CursorSdkEvent{}, fmt.Errorf("decode cursor sdk models event: %w", err)
		}
		return CursorSdkEvent{Event: evt.Event, Items: evt.Items}, nil
	case cursorSdkEventError:
		var evt CursorSdkErrorEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			return CursorSdkEvent{}, fmt.Errorf("decode cursor sdk error event: %w", err)
		}
		return CursorSdkEvent{
			Event:     evt.Event,
			Message:   evt.Message,
			Retryable: evt.Retryable,
		}, nil
	case cursorSdkEventResult:
		var evt CursorSdkResultEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			return CursorSdkEvent{}, fmt.Errorf("decode cursor sdk result event: %w", err)
		}
		return CursorSdkEvent{
			Event:        evt.Event,
			ResultStatus: evt.Status,
			ResultOutput: evt.Output,
			ResultUsage:  evt.Usage,
			ResultError:  evt.Error,
		}, nil
	default:
		return CursorSdkEvent{}, fmt.Errorf("decode cursor sdk event: unknown event %q", base.Event)
	}
}

// CursorSdkAgentIDEvent is emitted after create or resume.
type CursorSdkAgentIDEvent struct {
	Event   string `json:"event"`
	AgentID string `json:"agentId"`
}

// CursorSdkMessageEvent mirrors executor protocol message events.
type CursorSdkMessageEvent struct {
	Event   string          `json:"event"`
	Type    string          `json:"type"`
	Content string          `json:"content,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	CallID  string          `json:"callId,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Output  string          `json:"output,omitempty"`
	Status  string          `json:"status,omitempty"`
	Usage   json.RawMessage `json:"usage,omitempty"`
}

// CursorSdkModelsEvent carries the model catalog from list-models.
type CursorSdkModelsEvent struct {
	Event string            `json:"event"`
	Items []json.RawMessage `json:"items"`
}

// CursorSdkErrorEvent reports a non-terminal executor error.
type CursorSdkErrorEvent struct {
	Event     string `json:"event"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// CursorSdkResultEvent is the terminal run outcome.
type CursorSdkResultEvent struct {
	Event  string          `json:"event"`
	Status string          `json:"status"`
	Output string          `json:"output,omitempty"`
	Usage  json.RawMessage `json:"usage,omitempty"`
	Error  string          `json:"error,omitempty"`
}
