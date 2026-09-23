package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const (
	cursorSdkSandboxEnabledEnv   = "CURSOR_SDK_SANDBOX_ENABLED"
	cursorSdkCustomToolsJSONEnv  = "CURSOR_SDK_CUSTOM_TOOLS_JSON"
	cursorSdkDefaultAPIKeyEnv    = "CURSOR_API_KEY"
)

// cursorSdkBackend implements Backend by spawning the Node cursor-sdk executor
// and mapping its JSONL IPC events onto agent.Message and Result.
type cursorSdkBackend struct {
	cfg Config
}

func (b *cursorSdkBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error) {
	timeout := opts.Timeout
	runCtx, cancel := runContext(ctx, timeout)

	client, err := NewCursorSdkClient(runCtx, b.cfg.ExecutablePath, b.cfg.Logger)
	if err != nil {
		cancel()
		return nil, err
	}

	b.cfg.Logger.Info("cursor sdk executor connected", "cwd", opts.Cwd, "model", opts.Model)

	msgCh := make(chan Message, 256)
	resCh := make(chan Result, 1)

	var runRunning atomic.Bool
	reloadAfterMcpRefresh := opts.McpConfigRefreshed
	var reloadedAfterMcpRefresh atomic.Bool

	supplement := func(supplementCtx context.Context, instruction string) error {
		if !runRunning.Load() {
			return errors.New("cursor sdk run has not started")
		}
		return client.Steer(supplementCtx, instruction)
	}
	supplementReady := func() bool {
		return runRunning.Load()
	}

	go func() {
		defer cancel()
		defer close(msgCh)
		defer close(resCh)
		defer runRunning.Store(false)
		defer func() {
			if err := client.Close(); err != nil {
				b.cfg.Logger.Warn("cursor sdk executor close failed", "error", err)
			}
		}()

		startTime := time.Now()
		configuredModel := strings.TrimSpace(opts.Model)
		req, err := buildCursorSdkExecuteRequest(prompt, opts, b.cfg.Env)
		if err != nil {
			resCh <- Result{
				Status:     "failed",
				Error:      sanitizeAgentDiagnostic(err.Error()),
				DurationMs: time.Since(startTime).Milliseconds(),
			}
			return
		}

		var output strings.Builder
		sessionID := ""
		finalStatus := "completed"
		var finalError string
		resultSeen := false
		var resultUsage map[string]TokenUsage

		sdkResult, execErr := client.Execute(runCtx, req, func(evt CursorSdkEvent) {
			switch evt.Event {
			case cursorSdkEventAgentID:
				sessionID = strings.TrimSpace(evt.AgentID)
				if reloadAfterMcpRefresh && reloadedAfterMcpRefresh.CompareAndSwap(false, true) {
					reloadCtx := context.WithoutCancel(runCtx)
					if err := client.Reload(reloadCtx); err != nil {
						b.cfg.Logger.Warn("cursor sdk reload after mcp refresh failed", "error", err)
					}
				}
			case cursorSdkEventMessage:
				if msg, ok := cursorSdkMessageFromEvent(evt); ok {
					if msg.Type == MessageStatus && msg.Status == "running" {
						runRunning.Store(true)
					}
					if msg.Type == MessageText {
						output.WriteString(msg.Content)
					}
					trySend(msgCh, msg)
				}
			case cursorSdkEventError:
				errMsg := strings.TrimSpace(evt.Message)
				if errMsg != "" {
					trySend(msgCh, Message{Type: MessageError, Content: errMsg})
				}
			}
		})

		duration := time.Since(startTime)
		if execErr != nil {
			switch {
			case runCtx.Err() == context.DeadlineExceeded:
				finalStatus = "timeout"
				finalError = fmt.Sprintf("cursor sdk timed out after %s", timeout)
			case runCtx.Err() == context.Canceled:
				finalStatus = "aborted"
				finalError = "execution cancelled"
			default:
				finalStatus = "failed"
				finalError = execErr.Error()
			}
		} else {
			resultSeen = true
			if sessionID == "" {
				sessionID = strings.TrimSpace(sdkResult.AgentID)
			}
			switch strings.TrimSpace(sdkResult.Status) {
			case "completed", "finished":
				finalStatus = "completed"
			case "cancelled", "canceled", "aborted":
				finalStatus = "aborted"
			case "failed", "error":
				finalStatus = "failed"
			default:
				if sdkResult.Status != "" {
					finalStatus = sdkResult.Status
				}
			}
			if sdkResult.Output != "" && output.Len() == 0 {
				output.WriteString(sdkResult.Output)
				trySend(msgCh, Message{Type: MessageText, Content: sdkResult.Output})
			}
			if finalStatus == "failed" {
				finalError = strings.TrimSpace(sdkResult.Error)
				if finalError == "" {
					finalError = "cursor sdk returned an error result without details"
				}
			}
			resultUsage = cursorSdkUsageFromResult(sdkResult.Usage, configuredModel)
		}

		if finalError != "" {
			finalError = sanitizeAgentDiagnostic(finalError)
		}

		b.cfg.Logger.Info("cursor sdk finished",
			"status", finalStatus,
			"duration", duration.Round(time.Millisecond).String(),
			"result_seen", resultSeen,
		)

		finalOutput := output.String()
		if finalStatus != "completed" {
			finalOutput = ""
		}

		resCh <- Result{
			Status:     finalStatus,
			Output:     finalOutput,
			Error:      finalError,
			DurationMs: duration.Milliseconds(),
			SessionID:  sessionID,
			Usage:      resultUsage,
		}
	}()

	return &Session{
		Supplement:      supplement,
		SupplementReady: supplementReady,
		Messages:        msgCh,
		Result:          resCh,
	}, nil
}

func buildCursorSdkExecuteRequest(prompt string, opts ExecOptions, env map[string]string) (CursorSdkExecuteRequest, error) {
	req := CursorSdkExecuteRequest{
		Prompt:    prompt,
		Cwd:       opts.Cwd,
		Model:     opts.Model,
		AgentID:   opts.ResumeSessionID,
		APIKeyEnv: cursorSdkDefaultAPIKeyEnv,
	}

	if len(opts.McpConfig) > 0 {
		var mcpConfig map[string]any
		if err := json.Unmarshal(opts.McpConfig, &mcpConfig); err != nil {
			return CursorSdkExecuteRequest{}, fmt.Errorf("decode cursor sdk mcp config: %w", err)
		}
		req.McpConfig = mcpConfig
	}

	if cursorSdkEnvTruthy(cursorSdkEnvValue(env, cursorSdkSandboxEnabledEnv)) {
		req.SandboxOptions = map[string]any{"enabled": true}
	}

	if raw := cursorSdkEnvValue(env, cursorSdkCustomToolsJSONEnv); raw != "" {
		var customTools map[string]any
		if err := json.Unmarshal([]byte(raw), &customTools); err != nil {
			return CursorSdkExecuteRequest{}, fmt.Errorf("decode %s: %w", cursorSdkCustomToolsJSONEnv, err)
		}
		req.CustomTools = customTools
	}

	return req, nil
}

func cursorSdkEnvValue(env map[string]string, key string) string {
	if env != nil {
		if value := strings.TrimSpace(env[key]); value != "" {
			return value
		}
	}
	return strings.TrimSpace(os.Getenv(key))
}

func cursorSdkEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func cursorSdkMessageFromEvent(evt CursorSdkEvent) (Message, bool) {
	switch evt.MessageType {
	case "assistant":
		if strings.TrimSpace(evt.Content) == "" {
			return Message{}, false
		}
		return Message{Type: MessageText, Content: evt.Content}, true
	case "thinking":
		if strings.TrimSpace(evt.Content) == "" {
			return Message{}, false
		}
		return Message{Type: MessageThinking, Content: evt.Content}, true
	case "tool_use":
		var input map[string]any
		if len(evt.Input) > 0 {
			_ = json.Unmarshal(evt.Input, &input)
		}
		return Message{
			Type:   MessageToolUse,
			Tool:   evt.Tool,
			CallID: evt.CallID,
			Input:  input,
		}, true
	case "tool_result":
		return Message{
			Type:   MessageToolResult,
			Tool:   evt.Tool,
			CallID: evt.CallID,
			Output: evt.Output,
		}, true
	case "status":
		return Message{Type: MessageStatus, Status: evt.Status}, true
	case "error":
		return Message{Type: MessageError, Content: evt.Content}, true
	default:
		return Message{}, false
	}
}

func cursorSdkUsageFromResult(raw json.RawMessage, configuredModel string) map[string]TokenUsage {
	if len(raw) == 0 {
		return nil
	}

	var payload struct {
		Model            string       `json:"model"`
		InputTokens      int64        `json:"inputTokens"`
		OutputTokens     int64        `json:"outputTokens"`
		CacheReadTokens  int64        `json:"cacheReadTokens"`
		CacheWriteTokens int64        `json:"cacheWriteTokens"`
		Usage            *cursorUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	evt := cursorStreamEvent{
		Model:            payload.Model,
		InputTokens:      payload.InputTokens,
		OutputTokens:     payload.OutputTokens,
		CacheReadTokens:  payload.CacheReadTokens,
		CacheWriteTokens: payload.CacheWriteTokens,
		Usage:            payload.Usage,
	}
	if !evt.hasResultUsage() {
		return nil
	}

	usage := make(map[string]TokenUsage)
	backend := cursorBackend{}
	backend.accumulateResultUsage(usage, &evt, configuredModel)
	if len(usage) == 0 {
		return nil
	}
	return usage
}
