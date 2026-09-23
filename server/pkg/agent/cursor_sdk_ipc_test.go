package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"
)

const fakeCursorSdkExecutorScript = `
const readline = require("node:readline");

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });

rl.on("line", (line) => {
  const trimmed = line.trim();
  if (!trimmed) {
    return;
  }

  let cmd;
  try {
    cmd = JSON.parse(trimmed);
  } catch {
    return;
  }

  switch (cmd.cmd) {
    case "execute":
      process.stdout.write(JSON.stringify({ event: "agent_id", agentId: "agent-test-1" }) + "\n");
      process.stdout.write(
        JSON.stringify({ event: "message", type: "assistant", content: "hello from sdk" }) + "\n",
      );
      process.stdout.write(
        JSON.stringify({
          event: "result",
          status: "completed",
          output: "hello from sdk",
          usage: { inputTokens: 3, outputTokens: 5 },
        }) + "\n",
      );
      break;
    case "steer":
      if (cmd.text !== "steer-ok") {
        process.stdout.write(
          JSON.stringify({ event: "error", message: "unexpected steer text", retryable: false }) + "\n",
        );
      }
      break;
    case "cancel":
    case "reload":
      break;
    case "list-models":
      process.stdout.write(
        JSON.stringify({
          event: "models",
          items: [{ id: "composer-2", name: "Composer 2" }],
        }) + "\n",
      );
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
      break;
    default:
      process.stdout.write(
        JSON.stringify({ event: "error", message: "unknown command", retryable: false }) + "\n",
      );
  }
});
`

func writeFakeCursorSdkExecutor(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-cursor-sdk-executor.js")
	if err := os.WriteFile(script, []byte(fakeCursorSdkExecutorScript), 0o644); err != nil {
		t.Fatalf("write fake cursor sdk executor: %v", err)
	}
	return script
}

func TestCursorSdkIPC(t *testing.T) {
	t.Parallel()

	script := writeFakeCursorSdkExecutor(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewCursorSdkClient(ctx, script, slog.Default())
	if err != nil {
		t.Fatalf("NewCursorSdkClient: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	t.Run("Execute", func(t *testing.T) {
		var events []CursorSdkEvent
		result, err := client.Execute(ctx, CursorSdkExecuteRequest{
			Prompt: "say hello",
			Cwd:    t.TempDir(),
			Model:  "composer-2",
		}, func(evt CursorSdkEvent) {
			events = append(events, evt)
		})
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.Status != "completed" {
			t.Fatalf("result status = %q, want completed", result.Status)
		}
		if result.Output != "hello from sdk" {
			t.Fatalf("result output = %q, want hello from sdk", result.Output)
		}
		if result.AgentID != "agent-test-1" {
			t.Fatalf("result agent id = %q, want agent-test-1", result.AgentID)
		}
		if len(result.Usage) == 0 {
			t.Fatal("result usage missing")
		}

		var sawAgentID, sawAssistant, sawResult bool
		for _, evt := range events {
			switch evt.Event {
			case cursorSdkEventAgentID:
				sawAgentID = evt.AgentID == "agent-test-1"
			case cursorSdkEventMessage:
				sawAssistant = evt.MessageType == "assistant" && strings.Contains(evt.Content, "hello from sdk")
			case cursorSdkEventResult:
				sawResult = evt.ResultStatus == "completed"
			}
		}
		if !sawAgentID || !sawAssistant || !sawResult {
			t.Fatalf("events = %#v, want agent_id, assistant message, and result", events)
		}
	})

	t.Run("Steer", func(t *testing.T) {
		if err := client.Steer(ctx, "steer-ok"); err != nil {
			t.Fatalf("Steer: %v", err)
		}
	})

	t.Run("ListModels", func(t *testing.T) {
		items, err := client.ListModels(ctx, "")
		if err != nil {
			t.Fatalf("ListModels: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("models items = %d, want 1", len(items))
		}
		var model struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(items[0], &model); err != nil {
			t.Fatalf("decode model item: %v", err)
		}
		if model.ID != "composer-2" || model.Name != "Composer 2" {
			t.Fatalf("model = %#v, want composer-2", model)
		}
	})
}

func TestParseCursorSdkEvent(t *testing.T) {
	t.Parallel()

	evt, err := parseCursorSdkEvent([]byte(`{"event":"agent_id","agentId":"agent-1"}`))
	if err != nil {
		t.Fatalf("parse agent_id: %v", err)
	}
	if evt.Event != cursorSdkEventAgentID || evt.AgentID != "agent-1" {
		t.Fatalf("event = %#v", evt)
	}

	evt, err = parseCursorSdkEvent([]byte(`{"event":"message","type":"tool_use","tool":"grep","callId":"c1","input":{"pattern":"foo"}}`))
	if err != nil {
		t.Fatalf("parse tool_use: %v", err)
	}
	if evt.MessageType != "tool_use" || evt.Tool != "grep" || evt.CallID != "c1" {
		t.Fatalf("event = %#v", evt)
	}
}
