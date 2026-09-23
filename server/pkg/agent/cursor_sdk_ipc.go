package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
)

const cursorSdkExecutorEnv = "MULTICA_CURSOR_SDK_EXECUTOR"

// CursorSdkExecutorEnv overrides the bundled cursor-sdk executor script path.
const CursorSdkExecutorEnv = cursorSdkExecutorEnv

// DefaultCursorSdkExecutorRelPath is the bundled executor script relative to the
// repository root when CursorSdkExecutorEnv is unset.
var DefaultCursorSdkExecutorRelPath = filepath.Join("fork", "packages", "cursor-sdk-executor", "dist", "cli.js")

var defaultCursorSdkExecutorRelPath = DefaultCursorSdkExecutorRelPath

// CursorSdkClient communicates with the Node cursor-sdk executor over JSONL.
type CursorSdkClient struct {
	mu     sync.Mutex
	stdin  io.WriteCloser
	logger *slog.Logger

	cmd      *exec.Cmd
	cancel   context.CancelFunc
	readDone chan struct{}
	readErr  error

	nextID atomic.Uint64

	executeMu sync.Mutex
	onEvent   func(CursorSdkEvent)
	execDone  chan struct{}
	execErr   error

	listModelsMu sync.Mutex
	listModelsCh chan CursorSdkEvent
}

// NewCursorSdkClient spawns the Node executor and starts its stdout reader.
func NewCursorSdkClient(ctx context.Context, executorPath string, logger *slog.Logger) (*CursorSdkClient, error) {
	if logger == nil {
		logger = slog.Default()
	}

	node, script, err := resolveCursorSdkExecutor(executorPath)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, node, script)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("cursor sdk stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return nil, fmt.Errorf("cursor sdk stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		_ = stdin.Close()
		return nil, fmt.Errorf("cursor sdk stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdin.Close()
		return nil, fmt.Errorf("start cursor sdk executor: %w", err)
	}

	client := &CursorSdkClient{
		stdin:    stdin,
		logger:   logger,
		cmd:      cmd,
		cancel:   cancel,
		readDone: make(chan struct{}),
	}
	go client.copyStderr(stderr)
	go client.readStdout(stdout)

	logger.Info("cursor sdk executor started", "pid", cmd.Process.Pid, "script", script)
	return client, nil
}

// ResolveCursorSdkExecutor resolves the node binary and executor script path.
func ResolveCursorSdkExecutor(executorPath string) (node string, script string, err error) {
	return resolveCursorSdkExecutor(executorPath)
}

func resolveCursorSdkExecutor(executorPath string) (node string, script string, err error) {
	script = executorPath
	if script == "" {
		script = os.Getenv(cursorSdkExecutorEnv)
	}
	if script == "" {
		script = defaultCursorSdkExecutorRelPath
	}

	node, err = exec.LookPath("node")
	if err != nil {
		return "", "", fmt.Errorf("cursor sdk executor requires node on PATH: %w", err)
	}

	if _, statErr := os.Stat(script); statErr != nil {
		return "", "", fmt.Errorf("cursor sdk executor script not found at %q: %w", script, statErr)
	}

	return node, script, nil
}

func (c *CursorSdkClient) copyStderr(stderr io.Reader) {
	_, _ = io.Copy(newLogWriter(c.logger, "[cursor-sdk:stderr] "), stderr)
}

func (c *CursorSdkClient) readStdout(stdout io.Reader) {
	defer close(c.readDone)

	scanner := newAgentStreamScanner(stdout)
	for scanner.Scan() {
		evt, err := parseCursorSdkEvent(scanner.Bytes())
		if err != nil {
			c.readErr = err
			c.logger.Error("cursor sdk event parse failed", "error", err)
			return
		}
		c.dispatchEvent(evt)
	}
	if err := scanner.Err(); err != nil {
		c.readErr = err
	}
}

func (c *CursorSdkClient) dispatchEvent(evt CursorSdkEvent) {
	c.executeMu.Lock()
	onEvent := c.onEvent
	execDone := c.execDone
	c.executeMu.Unlock()

	if onEvent != nil {
		onEvent(evt)
	}

	switch evt.Event {
	case cursorSdkEventResult:
		if execDone != nil {
			c.executeMu.Lock()
			if c.execDone == execDone && c.execErr == nil {
				c.execErr = errCursorSdkExecuteDone
			}
			c.executeMu.Unlock()
			close(execDone)
		}
	case cursorSdkEventError:
		if execDone != nil {
			c.executeMu.Lock()
			if c.execDone == execDone && c.execErr == nil {
				c.execErr = fmt.Errorf("%s", evt.Message)
			}
			c.executeMu.Unlock()
			close(execDone)
		}
		c.listModelsMu.Lock()
		ch := c.listModelsCh
		c.listModelsMu.Unlock()
		if ch != nil {
			select {
			case ch <- evt:
			default:
			}
		}
	case cursorSdkEventModels:
		c.listModelsMu.Lock()
		ch := c.listModelsCh
		c.listModelsMu.Unlock()
		if ch != nil {
			select {
			case ch <- evt:
			default:
			}
		}
	}
}

var errCursorSdkExecuteDone = errors.New("cursor sdk execute completed")

func (c *CursorSdkClient) nextCommandID() string {
	return fmt.Sprintf("%d", c.nextID.Add(1))
}

func (c *CursorSdkClient) writeCommand(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal cursor sdk command: %w", err)
	}
	data = append(data, '\n')

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return errors.New("cursor sdk client closed")
	}
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write cursor sdk command: %w", err)
	}
	return nil
}

// Execute runs one agent turn and streams IPC events until a terminal result or error.
func (c *CursorSdkClient) Execute(ctx context.Context, req CursorSdkExecuteRequest, onEvent func(CursorSdkEvent)) (CursorSdkResult, error) {
	var result CursorSdkResult
	wrapped := func(evt CursorSdkEvent) {
		switch evt.Event {
		case cursorSdkEventAgentID:
			result.AgentID = evt.AgentID
		case cursorSdkEventResult:
			result.Status = evt.ResultStatus
			result.Output = evt.ResultOutput
			result.Usage = evt.ResultUsage
			result.Error = evt.ResultError
		}
		if onEvent != nil {
			onEvent(evt)
		}
	}

	c.executeMu.Lock()
	if c.execDone != nil {
		c.executeMu.Unlock()
		return CursorSdkResult{}, errors.New("cursor sdk execute already in progress")
	}
	execDone := make(chan struct{})
	c.onEvent = wrapped
	c.execDone = execDone
	c.execErr = nil
	c.executeMu.Unlock()

	defer func() {
		c.executeMu.Lock()
		c.onEvent = nil
		c.execDone = nil
		c.execErr = nil
		c.executeMu.Unlock()
	}()

	cmd := CursorSdkExecuteCommand{
		Cmd:            cursorSdkCommandExecute,
		ID:             c.nextCommandID(),
		Prompt:         req.Prompt,
		Cwd:            req.Cwd,
		Model:          req.Model,
		McpConfig:      req.McpConfig,
		CustomTools:    req.CustomTools,
		SandboxOptions: req.SandboxOptions,
		APIKeyEnv:      req.APIKeyEnv,
	}
	if req.AgentID != "" {
		agentID := req.AgentID
		cmd.AgentID = &agentID
	}

	if err := c.writeCommand(cmd); err != nil {
		return CursorSdkResult{}, err
	}

	cancelWatch := make(chan struct{})
	defer close(cancelWatch)
	go func() {
		select {
		case <-ctx.Done():
			cancelCtx := context.WithoutCancel(ctx)
			if err := c.Cancel(cancelCtx); err != nil {
				c.logger.Warn("cursor sdk cancel failed", "error", err)
			}
		case <-execDone:
		case <-cancelWatch:
		}
	}()

	select {
	case <-ctx.Done():
		select {
		case <-execDone:
		case <-c.readDone:
			if c.readErr != nil {
				return result, c.readErr
			}
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			return result, errors.New("cursor sdk executor exited during execute")
		}
	case <-execDone:
	case <-c.readDone:
		if c.readErr != nil {
			return result, c.readErr
		}
		return result, errors.New("cursor sdk executor exited during execute")
	}

	c.executeMu.Lock()
	execErr := c.execErr
	c.executeMu.Unlock()
	if execErr != nil && !errors.Is(execErr, errCursorSdkExecuteDone) {
		return result, execErr
	}

	return result, nil
}

// Steer sends supplemental text to the active executor run.
func (c *CursorSdkClient) Steer(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.writeCommand(CursorSdkSteerCommand{
		Cmd:  cursorSdkCommandSteer,
		ID:   c.nextCommandID(),
		Text: text,
	})
}

// Cancel asks the executor to cancel the active run.
func (c *CursorSdkClient) Cancel(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.writeCommand(CursorSdkCancelCommand{
		Cmd: cursorSdkCommandCancel,
		ID:  c.nextCommandID(),
	})
}

// Reload asks the executor to reload agent configuration.
func (c *CursorSdkClient) Reload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.writeCommand(CursorSdkReloadCommand{
		Cmd: cursorSdkCommandReload,
		ID:  c.nextCommandID(),
	})
}

// ListModels queries the executor for the authenticated model catalog.
func (c *CursorSdkClient) ListModels(ctx context.Context, apiKeyEnv string) ([]json.RawMessage, error) {
	ch := make(chan CursorSdkEvent, 1)
	c.listModelsMu.Lock()
	if c.listModelsCh != nil {
		c.listModelsMu.Unlock()
		return nil, errors.New("cursor sdk list-models already in progress")
	}
	c.listModelsCh = ch
	c.listModelsMu.Unlock()

	defer func() {
		c.listModelsMu.Lock()
		c.listModelsCh = nil
		c.listModelsMu.Unlock()
	}()

	if err := c.writeCommand(CursorSdkListModelsCommand{
		Cmd:       cursorSdkCommandListModels,
		ID:        c.nextCommandID(),
		APIKeyEnv: apiKeyEnv,
	}); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case evt := <-ch:
		switch evt.Event {
		case cursorSdkEventModels:
			return evt.Items, nil
		case cursorSdkEventError:
			return nil, fmt.Errorf("%s", evt.Message)
		default:
			return nil, fmt.Errorf("unexpected cursor sdk list-models event %q", evt.Event)
		}
	case <-c.readDone:
		if c.readErr != nil {
			return nil, c.readErr
		}
		return nil, errors.New("cursor sdk executor exited during list-models")
	}
}

// Close shuts down the executor process.
func (c *CursorSdkClient) Close() error {
	var closeErr error

	if err := c.writeCommand(CursorSdkShutdownCommand{
		Cmd: cursorSdkCommandShutdown,
		ID:  c.nextCommandID(),
	}); err != nil {
		closeErr = err
	}

	if c.cmd != nil && c.cmd.Process != nil {
		if waitErr := c.cmd.Wait(); waitErr != nil && closeErr == nil {
			closeErr = waitErr
		}
	}

	if c.cancel != nil {
		c.cancel()
	}

	<-c.readDone

	c.mu.Lock()
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}
	c.mu.Unlock()

	if c.readErr != nil && closeErr == nil {
		closeErr = c.readErr
	}
	return closeErr
}
