package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"
)

func TestParseCursorSdkModels(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"id":"composer-2.5","displayName":"Composer 2.5","default":true}`),
		json.RawMessage(`{"id":"auto","name":"Auto"}`),
		json.RawMessage(`{"id":"composer-2.5"}`),
	}
	models := parseCursorSdkModels(items)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "composer-2.5" || models[0].Label != "Composer 2.5" || !models[0].Default {
		t.Fatalf("unexpected first model: %#v", models[0])
	}
	if models[1].ID != "auto" || models[1].Label != "Auto" {
		t.Fatalf("unexpected second model: %#v", models[1])
	}
}

func TestDiscoverCursorSdkModels(t *testing.T) {
	t.Parallel()

	script := writeFakeCursorSdkExecutor(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	catalog, err := discoverCursorSdkModels(ctx, NewCommand(script, nil))
	if err != nil {
		t.Fatalf("discoverCursorSdkModels: %v", err)
	}
	if catalog.Fallback {
		t.Fatalf("expected live catalog, got fallback")
	}
	if len(catalog.Models) != 1 || catalog.Models[0].ID != "composer-2" {
		t.Fatalf("unexpected models: %#v", catalog.Models)
	}
}

func TestDiscoverCursorSdkModelsUsesDiscoveryEnv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "env-discovery-executor.js")
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
		t.Fatalf("write executor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	catalog, err := discoverCursorSdkModels(ctx, NewCommand(script, nil).WithDiscoveryEnv(map[string]string{
		"CURSOR_API_KEY": "from-agent-env",
	}))
	if err != nil {
		t.Fatalf("discoverCursorSdkModels: %v", err)
	}
	if catalog.Fallback {
		t.Fatalf("expected live catalog, got fallback")
	}
	if len(catalog.Models) != 1 || catalog.Models[0].ID != "from-agent-env" {
		t.Fatalf("unexpected models: %#v", catalog.Models)
	}
}

func TestDiscoverCursorSdkModelsMissingExecutorFallsBack(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	catalog, err := discoverCursorSdkModels(ctx, NewCommand(filepath.Join(t.TempDir(), "missing-cli.js"), nil))
	if err != nil {
		t.Fatalf("discoverCursorSdkModels: %v", err)
	}
	if !catalog.Fallback {
		t.Fatalf("expected fallback catalog")
	}
	if len(catalog.Models) != len(cursorStaticModels()) {
		t.Fatalf("expected static fallback models")
	}
}

func TestCursorSdkBackfillMessages(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"user","message":{"role":"user","content":"hi"}}`),
		json.RawMessage(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"prior answer"}]}}`),
	}
	messages := cursorSdkBackfillMessages(items)
	if len(messages) != 1 {
		t.Fatalf("expected 1 backfill message, got %d", len(messages))
	}
	if messages[0].Type != MessageText || messages[0].Content != "prior answer" {
		t.Fatalf("unexpected backfill message: %#v", messages[0])
	}
}

func TestCursorSdkResumeFailureBackfillsMessages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "resume-backfill-executor.js")
	const resumeBackfillScript = `
const readline = require("node:readline");
const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on("line", (line) => {
  const cmd = JSON.parse(line.trim());
  switch (cmd.cmd) {
    case "execute":
      process.stdout.write(JSON.stringify({ event: "error", message: "Agent agent-old not found", retryable: false }) + "\n");
      break;
    case "messages-list":
      process.stdout.write(JSON.stringify({
        event: "messages",
        items: [{ type: "assistant", message: { role: "assistant", content: [{ type: "text", text: "recovered history" }] } }],
      }) + "\n");
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
  }
});
`
	if err := os.WriteFile(script, []byte(resumeBackfillScript), 0o644); err != nil {
		t.Fatalf("write executor: %v", err)
	}

	backend, err := New("cursor_sdk", Config{
		ExecutablePath: script,
		Logger:         slog.Default(),
	})
	if err != nil {
		t.Fatalf("New(cursor_sdk): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := backend.Execute(ctx, "continue", ExecOptions{
		Cwd:             t.TempDir(),
		Model:           "composer-2",
		ResumeSessionID: "agent-old",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var messages []Message
	for msg := range session.Messages {
		messages = append(messages, msg)
	}
	result := <-session.Result

	if result.Status != "failed" {
		t.Fatalf("expected failed result, got %#v", result)
	}
	var backfilled string
	for _, msg := range messages {
		if msg.Type == MessageText && msg.Content == "recovered history" {
			backfilled = msg.Content
		}
	}
	if backfilled == "" {
		t.Fatalf("expected backfilled message in %#v", messages)
	}
}

func TestCursorSdkResumeFailureRetriesFreshAgent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "resume-retry-executor.js")
	const resumeRetryScript = `
const readline = require("node:readline");
const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on("line", (line) => {
  const cmd = JSON.parse(line.trim());
  switch (cmd.cmd) {
    case "execute":
      if (cmd.agentId) {
        process.stdout.write(JSON.stringify({ event: "error", message: "Agent agent-old not found", retryable: false }) + "\n");
        break;
      }
      if (!cmd.prompt.includes("continuity notice")) {
        process.stdout.write(JSON.stringify({ event: "error", message: "missing continuity notice", retryable: false }) + "\n");
        break;
      }
      process.stdout.write(JSON.stringify({ event: "agent_id", agentId: "agent-fresh" }) + "\n");
      process.stdout.write(JSON.stringify({ event: "result", status: "completed", output: "fresh start" }) + "\n");
      break;
    case "messages-list":
      process.stdout.write(JSON.stringify({ event: "messages", items: [] }) + "\n");
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
  }
});
`
	if err := os.WriteFile(script, []byte(resumeRetryScript), 0o644); err != nil {
		t.Fatalf("write executor: %v", err)
	}

	backend, err := New("cursor_sdk", Config{
		ExecutablePath: script,
		Logger:         slog.Default(),
	})
	if err != nil {
		t.Fatalf("New(cursor_sdk): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := backend.Execute(ctx, "continue task", ExecOptions{
		Cwd:                    t.TempDir(),
		Model:                  "composer-2",
		ResumeSessionID:        "agent-old",
		ResumeExpected:         true,
		ResumeContinuityNotice: "continuity notice\n\n",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for range session.Messages {
	}
	result := <-session.Result

	if result.Status != "completed" {
		t.Fatalf("expected completed retry, got %#v", result)
	}
	if result.SessionID != "agent-fresh" {
		t.Fatalf("session id = %q, want agent-fresh", result.SessionID)
	}
	if result.Output != "fresh start" {
		t.Fatalf("output = %q, want fresh start", result.Output)
	}
}
