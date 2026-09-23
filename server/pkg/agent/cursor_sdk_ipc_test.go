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
      process.stdout.write("19:50:37.978 INFO LocalCursorRulesService load completed meta={durationMs: 98}\n");
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
    case "messages-list":
      process.stdout.write(
        JSON.stringify({
          event: "messages",
          items: [{ type: "assistant", message: { role: "assistant", content: [{ type: "text", text: "prior turn" }] } }],
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

	client, err := NewCursorSdkClient(ctx, script, nil, slog.Default())
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

	t.Run("MessagesList", func(t *testing.T) {
		items, err := client.MessagesList(ctx, CursorSdkMessagesListRequest{
			AgentID: "agent-test-1",
			Cwd:     t.TempDir(),
		})
		if err != nil {
			t.Fatalf("MessagesList: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("messages items = %d, want 1", len(items))
		}
		messages := cursorSdkBackfillMessages(items)
		if len(messages) != 1 || messages[0].Content != "prior turn" {
			t.Fatalf("backfill messages = %#v", messages)
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

	evt, err = parseCursorSdkEvent([]byte(`{"event":"messages","items":[{"type":"assistant"}]}`))
	if err != nil {
		t.Fatalf("parse messages: %v", err)
	}
	if evt.Event != cursorSdkEventMessages || len(evt.Items) != 1 {
		t.Fatalf("event = %#v", evt)
	}
}

func TestCursorSdkExecutorInheritsAgentEnv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "env-cursor-sdk-executor.js")
	const envScript = `
const readline = require("node:readline");
const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on("line", (line) => {
  const cmd = JSON.parse(line.trim());
  switch (cmd.cmd) {
    case "list-models":
      process.stdout.write(JSON.stringify({
        event: "models",
        items: [{ id: process.env.CURSOR_API_KEY || "missing" }],
      }) + "\n");
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
  }
});
`
	if err := os.WriteFile(script, []byte(envScript), 0o644); err != nil {
		t.Fatalf("write env executor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewCursorSdkClient(ctx, script, map[string]string{
		"CURSOR_API_KEY": "from-agent-env",
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewCursorSdkClient: %v", err)
	}
	defer func() {
		_ = client.Close()
	}()

	items, err := client.ListModels(ctx, cursorSdkDefaultAPIKeyEnv)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	var model struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(items[0], &model); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	if model.ID != "from-agent-env" {
		t.Fatalf("model id = %q, want from-agent-env", model.ID)
	}
}

func TestLocateCursorSdkExecutorScriptWalksUpToRepo(t *testing.T) {
	repoRoot := t.TempDir()
	rel := filepath.Join("fork", "packages", "cursor-sdk-executor", "dist", "cli.js")
	scriptPath := filepath.Join(repoRoot, rel)
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir executor dist: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("// stub"), 0o644); err != nil {
		t.Fatalf("write executor stub: %v", err)
	}

	subdir := filepath.Join(repoRoot, "server", "cmd", "server")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir nested cwd: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(subdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Setenv(cursorSdkExecutorEnv, "")

	got, err := locateCursorSdkExecutorScript("")
	if err != nil {
		t.Fatalf("locateCursorSdkExecutorScript: %v", err)
	}
	want, err := filepath.Abs(scriptPath)
	if err != nil {
		t.Fatalf("abs script path: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("script = %q, want %q", got, want)
	}
}

func TestLocateCursorSdkExecutorScriptExplicitMissingDoesNotFallback(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-cli.js")
	t.Setenv(cursorSdkExecutorEnv, missing)

	_, err := locateCursorSdkExecutorScript("")
	if err == nil {
		t.Fatal("expected error for missing MULTICA_CURSOR_SDK_EXECUTOR")
	}
}

func TestLocateCursorSdkExecutorScriptPrefersExplicitPath(t *testing.T) {
	t.Parallel()

	script := writeFakeCursorSdkExecutor(t)
	got, err := locateCursorSdkExecutorScript(script)
	if err != nil {
		t.Fatalf("locateCursorSdkExecutorScript: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(script) {
		t.Fatalf("script = %q, want %q", got, script)
	}
}

func TestCursorSdkMessagesListError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "messages-list-error-executor.js")
	const messagesListErrorScript = `
const readline = require("node:readline");
const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on("line", (line) => {
  const cmd = JSON.parse(line.trim());
  switch (cmd.cmd) {
    case "messages-list":
      process.stdout.write(JSON.stringify({ event: "error", message: "transcript unavailable", retryable: false }) + "\n");
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
  }
});
`
	if err := os.WriteFile(script, []byte(messagesListErrorScript), 0o644); err != nil {
		t.Fatalf("write messages-list error executor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewCursorSdkClient(ctx, script, nil, slog.Default())
	if err != nil {
		t.Fatalf("NewCursorSdkClient: %v", err)
	}
	defer func() {
		_ = client.Close()
	}()

	_, err = client.MessagesList(ctx, CursorSdkMessagesListRequest{
		AgentID: "agent-old",
		Cwd:     t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected messages-list error")
	}
	if !strings.Contains(err.Error(), "transcript unavailable") {
		t.Fatalf("error = %q, want transcript unavailable", err.Error())
	}
}
