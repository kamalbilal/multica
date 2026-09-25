package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	cursorSdkSandboxEnabledEnv   = "CURSOR_SDK_SANDBOX_ENABLED"
	cursorSdkCustomToolsJSONEnv  = "CURSOR_SDK_CUSTOM_TOOLS_JSON"
	cursorSdkDefaultAPIKeyEnv    = "CURSOR_API_KEY"
	// After a successful `multica issue comment add`, wait this long with no
	// further model activity before cancelling the SDK run. Immediate cancel
	// treats a progress comment as the final deliverable and also drops a
	// supplement that arrives while that comment is in flight.
	cursorSdkCommentDeliveryIdleWait = 8 * time.Second
)

// cursorSdkBackend implements Backend by spawning the Node cursor-sdk executor
// and mapping its JSONL IPC events onto agent.Message and Result.
type cursorSdkBackend struct {
	cfg             Config
	commentIdleWait time.Duration
}

func (b *cursorSdkBackend) commentDeliveryIdleWait() time.Duration {
	if b.commentIdleWait > 0 {
		return b.commentIdleWait
	}
	return cursorSdkCommentDeliveryIdleWait
}

// commentDeliveryWatchdog cancels a hung SDK run after the last issue comment,
// but only once the model has gone idle. New tokens, tools, or a steer reset
// the timer so a progress comment plus more work can finish.
type commentDeliveryWatchdog struct {
	mu    sync.Mutex
	timer *time.Timer
	armed bool
	fired atomic.Bool
}

func (w *commentDeliveryWatchdog) arm(idle time.Duration, fire func()) {
	if idle <= 0 || fire == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fired.Load() {
		return
	}
	w.armed = true
	if w.timer == nil {
		w.timer = time.AfterFunc(idle, func() {
			if w.fired.CompareAndSwap(false, true) {
				fire()
			}
		})
		return
	}
	w.timer.Reset(idle)
}

func (w *commentDeliveryWatchdog) ping(idle time.Duration) {
	if idle <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.armed || w.fired.Load() || w.timer == nil {
		return
	}
	w.timer.Reset(idle)
}

func (w *commentDeliveryWatchdog) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.armed = false
	if w.timer != nil {
		w.timer.Stop()
	}
}

func (w *commentDeliveryWatchdog) reset() {
	w.stop()
	w.fired.Store(false)
}

func (b *cursorSdkBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error) {
	timeout := opts.Timeout
	runCtx, cancel := runContext(ctx, timeout)

	client, err := NewCursorSdkClient(runCtx, b.cfg.ExecutablePath, b.cfg.Env, b.cfg.Logger)
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
	var commentWatchdog commentDeliveryWatchdog
	idleWait := b.commentDeliveryIdleWait()

	supplement := func(supplementCtx context.Context, instruction string) error {
		if !runRunning.Load() {
			return errors.New("cursor sdk run has not started")
		}
		commentWatchdog.ping(idleWait)
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
		defer commentWatchdog.stop()
		defer func() {
			if err := client.Close(); err != nil {
				b.cfg.Logger.Warn("cursor sdk executor close failed", "error", err)
			}
		}()

		startTime := time.Now()
		configuredModel := strings.TrimSpace(opts.Model)

		var output strings.Builder
		sessionID := ""
		finalStatus := "completed"
		var finalError string
		resultSeen := false
		var resultUsage map[string]TokenUsage

		turnPrompt := prompt
		attemptOpts := opts
		resumeSessionForBackfill := strings.TrimSpace(opts.ResumeSessionID)
		var execErr error
		var sdkResult CursorSdkResult
		var issueCommentCallIDs sync.Map
		var deliveryCompleted atomic.Bool

		for attempt := 0; attempt < 2; attempt++ {
			output.Reset()
			sessionID = ""
			resultSeen = false
			resultUsage = nil
			finalStatus = "completed"
			finalError = ""
			runRunning.Store(false)
			deliveryCompleted.Store(false)
			commentWatchdog.reset()

			req, buildErr := buildCursorSdkExecuteRequest(turnPrompt, attemptOpts, b.cfg.Env)
			if buildErr != nil {
				resCh <- Result{
					Status:     "failed",
					Error:      sanitizeAgentDiagnostic(buildErr.Error()),
					DurationMs: time.Since(startTime).Milliseconds(),
				}
				return
			}

			sdkResult, execErr = client.Execute(runCtx, req, func(evt CursorSdkEvent) {
				switch evt.Event {
				case cursorSdkEventAgentID:
					sessionID = strings.TrimSpace(evt.AgentID)
					runRunning.Store(true)
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
						if msg.Type == MessageToolUse && msg.CallID != "" && isMulticaIssueCommentAddTool(msg) {
							issueCommentCallIDs.Store(msg.CallID, struct{}{})
						}
						if msg.Type == MessageToolResult {
							_, tracked := issueCommentCallIDs.LoadAndDelete(msg.CallID)
							posted := (tracked && multicaIssueCommentAddToolSucceeded(msg)) ||
								multicaIssueCommentAddResultPosted(msg)
							if posted {
								b.cfg.Logger.Info("cursor sdk posted issue comment; waiting for idle before ending turn", "call_id", msg.CallID, "idle", idleWait)
								commentWatchdog.arm(idleWait, func() {
									deliveryCompleted.Store(true)
									b.cfg.Logger.Info("cursor sdk ending turn after issue comment idle", "call_id", msg.CallID)
									// Cancel the SDK run so the next issue comment can resume
									// this agent. Killing immediately leaves Cursor with an
									// active run and the follow-up fails with AgentBusyError.
									if err := client.CancelThenKillIfStuck(cursorSdkCommentDeliveryEndWait); err != nil {
										b.cfg.Logger.Warn("cursor sdk end after issue comment delivery failed", "error", err)
									}
								})
							} else {
								commentWatchdog.ping(idleWait)
							}
						} else {
							commentWatchdog.ping(idleWait)
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
			commentWatchdog.stop()

			if execErr == nil {
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

			shouldBackfill := resumeSessionForBackfill != "" && (execErr != nil || !resultSeen)
			if shouldBackfill {
				cursorSdkBackfillTranscript(runCtx, client, msgCh, b.cfg.Logger, resumeSessionForBackfill, opts.Cwd)
			}

			if attempt == 0 && execErr != nil && cursorSdkIsResumeError(execErr) && opts.ResumeExpected {
				b.cfg.Logger.Warn("cursor sdk resume failed; retrying with fresh agent",
					"prior_agent_id", attemptOpts.ResumeSessionID,
					"error", execErr,
				)
				attemptOpts.ResumeSessionID = ""
				turnPrompt = cursorSdkTurnPrompt(prompt, opts)
				continue
			}
			break
		}

		duration := time.Since(startTime)
		if deliveryCompleted.Load() {
			finalStatus = "completed"
			finalError = ""
			execErr = nil
		}
		if execErr != nil {
			switch {
			case runCtx.Err() == context.DeadlineExceeded:
				finalStatus = "timeout"
				finalError = fmt.Sprintf("cursor sdk timed out after %s", timeout)
			case runCtx.Err() == context.Canceled && deliveryCompleted.Load():
				finalStatus = "completed"
				finalError = ""
			case runCtx.Err() == context.Canceled:
				finalStatus = "aborted"
				finalError = "execution cancelled"
			default:
				finalStatus = "failed"
				finalError = execErr.Error()
			}
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
			Status: evt.Status,
		}, true
	case "status":
		return Message{Type: MessageStatus, Status: evt.Status}, true
	case "error":
		return Message{Type: MessageError, Content: evt.Content}, true
	default:
		return Message{}, false
	}
}

func cursorSdkToolResultSucceeded(msg Message) bool {
	switch strings.ToLower(strings.TrimSpace(msg.Status)) {
	case "failed", "error", "cancelled", "canceled", "aborted":
		return false
	default:
		return true
	}
}

func cursorSdkIsResumeError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "already has active run") {
		return true
	}
	if strings.Contains(lower, "agent not found") {
		return true
	}
	return strings.Contains(lower, "agent ") && strings.Contains(lower, " not found")
}

func cursorSdkTurnPrompt(prompt string, opts ExecOptions) string {
	notice := strings.TrimSpace(opts.ResumeContinuityNotice)
	if notice == "" {
		return prompt
	}
	return notice + prompt
}

func cursorSdkBackfillTranscript(
	ctx context.Context,
	client *CursorSdkClient,
	msgCh chan Message,
	logger *slog.Logger,
	agentID string,
	cwd string,
) {
	backfillCtx := context.WithoutCancel(ctx)
	items, listErr := client.MessagesList(backfillCtx, CursorSdkMessagesListRequest{
		AgentID:   agentID,
		Cwd:       cwd,
		APIKeyEnv: cursorSdkDefaultAPIKeyEnv,
	})
	if listErr != nil {
		logger.Warn("cursor sdk messages.list backfill failed", "error", listErr)
		return
	}
	for _, msg := range cursorSdkBackfillMessages(items) {
		trySend(msgCh, msg)
	}
}

func cursorSdkBackfillMessages(items []json.RawMessage) []Message {
	var messages []Message
	for _, raw := range items {
		var entry struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil || entry.Type != "assistant" {
			continue
		}
		text := cursorSdkExtractMessageText(entry.Message)
		if text == "" {
			continue
		}
		messages = append(messages, Message{Type: MessageText, Content: text})
	}
	return messages
}

func cursorSdkExtractMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if len(payload.Content) == 0 {
		return ""
	}
	switch payload.Content[0] {
	case '"':
		var text string
		if err := json.Unmarshal(payload.Content, &text); err == nil {
			return strings.TrimSpace(text)
		}
	case '[':
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(payload.Content, &blocks); err != nil {
			return ""
		}
		var parts []string
		for _, block := range blocks {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
	return ""
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
