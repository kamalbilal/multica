package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"
)

const mappingCursorSdkExecutorScript = `
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
      if (cmd.sandboxOptions?.enabled !== true) {
        process.stdout.write(
          JSON.stringify({ event: "error", message: "sandbox not enabled", retryable: false }) + "\n",
        );
        break;
      }
      if (!cmd.customTools?.probe) {
        process.stdout.write(
          JSON.stringify({ event: "error", message: "custom tools missing", retryable: false }) + "\n",
        );
        break;
      }
      process.stdout.write(JSON.stringify({ event: "agent_id", agentId: "agent-map-test" }) + "\n");
      process.stdout.write(JSON.stringify({ event: "message", type: "status", status: "running" }) + "\n");
      process.stdout.write(JSON.stringify({ event: "message", type: "thinking", content: "hmm" }) + "\n");
      process.stdout.write(JSON.stringify({ event: "message", type: "assistant", content: "hello sdk" }) + "\n");
      process.stdout.write(
        JSON.stringify({
          event: "message",
          type: "tool_use",
          tool: "grep",
          callId: "call-1",
          input: { pattern: "foo" },
        }) + "\n",
      );
      process.stdout.write(
        JSON.stringify({
          event: "message",
          type: "tool_result",
          tool: "grep",
          callId: "call-1",
          output: "matched",
        }) + "\n",
      );
      process.stdout.write(JSON.stringify({ event: "message", type: "error", content: "soft error" }) + "\n");
      process.stdout.write(
        JSON.stringify({
          event: "result",
          status: "completed",
          output: "hello sdk",
          usage: { inputTokens: 11, outputTokens: 7, cacheReadTokens: 2 },
        }) + "\n",
      );
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
      break;
    default:
      break;
  }
});
`

func TestCursorSdkExecuteMapsMessages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "mapping-cursor-sdk-executor.js")
	if err := os.WriteFile(script, []byte(mappingCursorSdkExecutorScript), 0o644); err != nil {
		t.Fatalf("write fake executor: %v", err)
	}

	backend, err := New("cursor_sdk", Config{
		ExecutablePath: script,
		Logger:         slog.Default(),
		Env: map[string]string{
			cursorSdkSandboxEnabledEnv:  "true",
			cursorSdkCustomToolsJSONEnv: `{"probe":true}`,
		},
	})
	if err != nil {
		t.Fatalf("New(cursor_sdk): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := backend.Execute(ctx, "do work", ExecOptions{
		Cwd:   t.TempDir(),
		Model: "composer-2",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var messages []Message
	for msg := range session.Messages {
		messages = append(messages, msg)
	}

	result, ok := readSessionResult(session.Result, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for cursor sdk result")
	}

	if result.SessionID != "agent-map-test" {
		t.Fatalf("session id = %q, want agent-map-test", result.SessionID)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed (error=%q)", result.Status, result.Error)
	}
	if result.Output != "hello sdk" {
		t.Fatalf("output = %q, want hello sdk", result.Output)
	}

	usage := result.Usage["composer-2"]
	if usage.InputTokens != 11 || usage.OutputTokens != 7 || usage.CacheReadTokens != 2 {
		t.Fatalf("usage = %#v, want input=11 output=7 cache_read=2", usage)
	}

	wantTypes := []MessageType{
		MessageStatus,
		MessageThinking,
		MessageText,
		MessageToolUse,
		MessageToolResult,
		MessageError,
	}
	if len(messages) < len(wantTypes) {
		t.Fatalf("messages = %#v, want at least %d mapped events", messages, len(wantTypes))
	}
	for i, want := range wantTypes {
		if messages[i].Type != want {
			t.Fatalf("messages[%d].Type = %q, want %q (messages=%#v)", i, messages[i].Type, want, messages)
		}
	}
	if messages[2].Content != "hello sdk" {
		t.Fatalf("assistant content = %q, want hello sdk", messages[2].Content)
	}
	if messages[3].Tool != "grep" || messages[3].CallID != "call-1" {
		t.Fatalf("tool_use = %#v", messages[3])
	}
	if messages[4].Output != "matched" {
		t.Fatalf("tool_result output = %q, want matched", messages[4].Output)
	}
}

func TestBuildCursorSdkExecuteRequestReadsEnv(t *testing.T) {
	t.Parallel()

	req, err := buildCursorSdkExecuteRequest("prompt", ExecOptions{
		Cwd:   "/tmp/work",
		Model: "composer-2",
	}, map[string]string{
		cursorSdkSandboxEnabledEnv:  "1",
		cursorSdkCustomToolsJSONEnv: `{"probe":{"description":"x"}}`,
	})
	if err != nil {
		t.Fatalf("buildCursorSdkExecuteRequest: %v", err)
	}
	if req.SandboxOptions["enabled"] != true {
		t.Fatalf("sandbox options = %#v", req.SandboxOptions)
	}
	if req.CustomTools["probe"] == nil {
		t.Fatalf("custom tools = %#v", req.CustomTools)
	}
}

func TestCursorSdkEnvTruthy(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "0", "false", "no", "off"} {
		if cursorSdkEnvTruthy(value) {
			t.Fatalf("cursorSdkEnvTruthy(%q) = true, want false", value)
		}
	}
	for _, value := range []string{"1", "true", "yes", "on"} {
		if !cursorSdkEnvTruthy(value) {
			t.Fatalf("cursorSdkEnvTruthy(%q) = false, want true", value)
		}
	}
}

func readSessionResult(ch <-chan Result, timeout time.Duration) (Result, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result, ok := <-ch:
		if !ok {
			return Result{}, false
		}
		return result, true
	case <-timer.C:
		return Result{}, false
	}
}

const supplementCursorSdkExecutorScript = `
const readline = require("node:readline");

let executeRunning = false;
let finishExecute = null;
const steerLines = [];

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });

function emit(event) {
  process.stdout.write(JSON.stringify(event) + "\n");
}

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
      executeRunning = true;
      emit({ event: "agent_id", agentId: "agent-supplement-test" });
      emit({ event: "message", type: "status", status: "running" });
      finishExecute = () => {
        executeRunning = false;
        emit({
          event: "result",
          status: "completed",
          output: "steered ok",
        });
      };
      break;
    case "steer":
      steerLines.push(cmd.text);
      if (finishExecute) {
        finishExecute();
        finishExecute = null;
      }
      break;
    case "reload":
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
      break;
    default:
      break;
  }
});
`

func TestCursorSdkSupplementSteers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "supplement-cursor-sdk-executor.js")
	if err := os.WriteFile(script, []byte(supplementCursorSdkExecutorScript), 0o644); err != nil {
		t.Fatalf("write fake executor: %v", err)
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

	session, err := backend.Execute(ctx, "do work", ExecOptions{
		Cwd:                  t.TempDir(),
		Model:                "composer-2",
		EnableTaskSupplement: true,
		McpConfigRefreshed:   true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if session.Supplement == nil || session.SupplementReady == nil {
		t.Fatal("cursor sdk session missing supplement hooks")
	}

	deadline := time.Now().Add(5 * time.Second)
	for !session.SupplementReady() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for supplement-ready cursor sdk run")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := session.Supplement(ctx, "keep the original goal and add this"); err != nil {
		t.Fatalf("Supplement: %v", err)
	}

	result, ok := readSessionResult(session.Result, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for cursor sdk result")
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed (error=%q)", result.Status, result.Error)
	}
	if result.Output != "steered ok" {
		t.Fatalf("output = %q, want steered ok", result.Output)
	}
}

func TestCursorSdkSupplementFailsClosedWithoutActiveRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "supplement-closed-cursor-sdk-executor.js")
	if err := os.WriteFile(script, []byte(supplementCursorSdkExecutorScript), 0o644); err != nil {
		t.Fatalf("write fake executor: %v", err)
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

	session, err := backend.Execute(ctx, "do work", ExecOptions{
		Cwd:   t.TempDir(),
		Model: "composer-2",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if session.Supplement == nil {
		t.Fatal("cursor sdk session missing supplement hook")
	}
	if err := session.Supplement(ctx, "extra"); err == nil {
		t.Fatal("Supplement succeeded without an active run")
	}
	cancel()
	readSessionResult(session.Result, 5*time.Second)
}

const reloadCursorSdkExecutorScript = `
const readline = require("node:readline");

let finishExecute = null;
let reloadCalled = false;

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });

function emit(event) {
  process.stdout.write(JSON.stringify(event) + "\n");
}

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
      emit({ event: "agent_id", agentId: "agent-reload-test" });
      emit({ event: "message", type: "status", status: "running" });
      finishExecute = () => {
        emit({
          event: "result",
          status: "completed",
          output: reloadCalled ? "reloaded" : "missing reload",
        });
      };
      break;
    case "reload":
      reloadCalled = true;
      if (finishExecute) {
        finishExecute();
        finishExecute = null;
      }
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
      break;
    default:
      break;
  }
});
`

func TestCursorSdkReloadAfterMcpRefresh(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	script := filepath.Join(dir, "reload-cursor-sdk-executor.js")
	if err := os.WriteFile(script, []byte(reloadCursorSdkExecutorScript), 0o644); err != nil {
		t.Fatalf("write reload executor: %v", err)
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

	session, err := backend.Execute(ctx, "do work", ExecOptions{
		Cwd:                t.TempDir(),
		Model:              "composer-2",
		McpConfigRefreshed: true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, ok := readSessionResult(session.Result, 5*time.Second)
	if !ok {
		t.Fatal("timed out waiting for cursor sdk reload result")
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed (error=%q)", result.Status, result.Error)
	}
	if result.Output != "reloaded" {
		t.Fatalf("output = %q, want reloaded", result.Output)
	}
}

func TestCursorSdkCancelDrainsResult(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cancelScript := `
const readline = require("node:readline");
const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
function emit(event) { process.stdout.write(JSON.stringify(event) + "\n"); }
rl.on("line", (line) => {
  const cmd = JSON.parse(line.trim());
  switch (cmd.cmd) {
    case "execute":
      emit({ event: "agent_id", agentId: "agent-cancel-test" });
      emit({ event: "message", type: "status", status: "running" });
      break;
    case "cancel":
      emit({ event: "result", status: "cancelled", output: "" });
      break;
    case "shutdown":
      rl.close();
      process.exit(0);
  }
});
`
	script := filepath.Join(dir, "cancel-cursor-sdk-executor.js")
	if err := os.WriteFile(script, []byte(cancelScript), 0o644); err != nil {
		t.Fatalf("write cancel executor: %v", err)
	}

	clientCtx, clientCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer clientCancel()
	client, err := NewCursorSdkClient(clientCtx, script, slog.Default())
	if err != nil {
		t.Fatalf("NewCursorSdkClient: %v", err)
	}
	defer func() {
		_ = client.Close()
	}()

	execCtx, cancelExec := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancelExec()
	}()

	result, err := client.Execute(execCtx, CursorSdkExecuteRequest{
		Prompt: "prompt",
		Cwd:    t.TempDir(),
		Model:  "composer-2",
	}, nil)
	if err != nil {
		t.Fatalf("Execute after cancel: %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("result status = %q, want cancelled", result.Status)
	}
}

func TestLaunchHeaderIncludesCursorSdk(t *testing.T) {
	t.Parallel()

	header := LaunchHeader("cursor_sdk")
	if !strings.Contains(header, "cursor-sdk") {
		t.Fatalf("LaunchHeader(cursor_sdk) = %q, want cursor-sdk prefix", header)
	}
}
