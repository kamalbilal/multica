import {
  Agent,
  AgentBusyError,
  Cursor,
  type AgentOptions,
  type McpServerConfig,
  type SDKAgent,
  type Run,
} from "@cursor/sdk";
import type {
  CancelCommand,
  ExecuteCommand,
  IpcCommand,
  ListModelsCommand,
  MessagesListCommand,
  ReloadCommand,
  SteerCommand,
} from "./protocol.js";
import { mapSdkMessage } from "./stream-mapper.js";

export type EmitFn = (event: unknown) => void;

const DEFAULT_SETTING_SOURCES = ["project"] as const;

interface ExecutorState {
  activeAgent: SDKAgent | null;
  activeRun: Run | null;
}

const state: ExecutorState = {
  activeAgent: null,
  activeRun: null,
};

const RUN_WAIT_TIMEOUT_MS = 15_000;
const RUN_WAIT_AFTER_CANCEL_MS = 2_000;
const CANCEL_TIMEOUT_MS = 2_000;

type ActiveExecuteControl = {
  abort: () => void;
};

let activeExecuteControl: ActiveExecuteControl | null = null;

function createAbortGate(): { signal: { aborted: boolean; wait: Promise<void> }; abort: () => void } {
  let aborted = false;
  let resolveWait: () => void = () => {};
  const wait = new Promise<void>((resolve) => {
    resolveWait = resolve;
  });
  return {
    signal: {
      get aborted() {
        return aborted;
      },
      wait,
    },
    abort() {
      if (aborted) {
        return;
      }
      aborted = true;
      resolveWait();
    },
  };
}

function resolveApiKey(apiKeyEnv?: string): string | undefined {
  const envName = apiKeyEnv ?? "CURSOR_API_KEY";
  const value = process.env[envName];
  return value?.trim() ? value : undefined;
}

function buildAgentOptions(cmd: ExecuteCommand, apiKey: string): AgentOptions {
  const local: NonNullable<AgentOptions["local"]> = {
    cwd: cmd.cwd,
    settingSources: [...DEFAULT_SETTING_SOURCES],
  };
  if (cmd.sandboxOptions) {
    local.sandboxOptions = cmd.sandboxOptions as unknown as NonNullable<
      NonNullable<AgentOptions["local"]>["sandboxOptions"]
    >;
  }
  if (cmd.customTools) {
    local.customTools = cmd.customTools as unknown as NonNullable<
      NonNullable<AgentOptions["local"]>["customTools"]
    >;
  }
  return {
    apiKey,
    model: { id: cmd.model },
    local,
    mcpServers: asMcpConfig(cmd.mcpConfig),
  };
}

function asMcpConfig(
  mcpConfig?: Record<string, unknown>,
): Record<string, McpServerConfig> | undefined {
  if (!mcpConfig) {
    return undefined;
  }
  return mcpConfig as Record<string, McpServerConfig>;
}

async function disposeAgent(agent: SDKAgent | null): Promise<void> {
  if (!agent) {
    return;
  }
  await agent[Symbol.asyncDispose]();
}

async function withTimeout<T>(promise: Promise<T>, ms: number, label: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      promise,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(new Error(label)), ms);
      }),
    ]);
  } finally {
    if (timer !== undefined) {
      clearTimeout(timer);
    }
  }
}

async function consumeRunStream(
  run: Run,
  emit: EmitFn,
  abortSignal: { aborted: boolean; wait: Promise<void> },
): Promise<void> {
  const iterator = run.stream()[Symbol.asyncIterator]();
  try {
    while (!abortSignal.aborted) {
      const next = await Promise.race([
        iterator.next(),
        abortSignal.wait.then(() => ({ done: true as const, value: undefined })),
      ]);
      if (next.done) {
        break;
      }
      const mapped = mapSdkMessage(next.value);
      if (mapped) {
        emit(mapped);
      }
    }
  } finally {
    void iterator.return?.();
  }
}

async function waitForRunResult(
  run: Run,
  aborted: boolean,
): Promise<Awaited<ReturnType<Run["wait"]>>> {
  const timeoutMs = aborted ? RUN_WAIT_AFTER_CANCEL_MS : RUN_WAIT_TIMEOUT_MS;
  try {
    return await withTimeout(run.wait(), timeoutMs, "run.wait timed out");
  } catch (err) {
    if (aborted) {
      return {
        id: run.id,
        status: "cancelled",
        result: "",
      } as Awaited<ReturnType<Run["wait"]>>;
    }
    throw err instanceof Error ? err : new Error("run.wait timed out");
  }
}

function isActiveRunStatus(status: string | undefined): boolean {
  const normalized = String(status ?? "").toLowerCase();
  return normalized === "running" || normalized === "creating";
}

function isAgentBusyError(err: unknown): boolean {
  if (err instanceof AgentBusyError) {
    return true;
  }
  const message = err instanceof Error ? err.message : String(err);
  return /already has active run/i.test(message);
}

async function cancelLeftoverActiveRuns(agentId: string, cwd: string): Promise<void> {
  let items: Run[] = [];
  try {
    const listed = await Agent.listRuns(agentId, { runtime: "local", cwd, limit: 20 });
    items = listed.items ?? [];
  } catch {
    return;
  }
  for (const run of items) {
    if (!isActiveRunStatus(run.status)) {
      continue;
    }
    try {
      if (run.supports("cancel")) {
        await withTimeout(Promise.resolve(run.cancel()), CANCEL_TIMEOUT_MS, "leftover run.cancel timed out");
      } else {
        await withTimeout(
          Agent.cancelRun(run.id, { runtime: "local", cwd }),
          CANCEL_TIMEOUT_MS,
          "leftover cancelRun timed out",
        );
      }
    } catch {
      // send() will surface AgentBusyError if the leftover run survived.
    }
  }
}

async function sendPrompt(agent: SDKAgent, cmd: ExecuteCommand, mcpServers: AgentOptions["mcpServers"]): Promise<Run> {
  try {
    return await agent.send(cmd.prompt, { mcpServers });
  } catch (err) {
    if (!cmd.agentId || !isAgentBusyError(err)) {
      throw err;
    }
    await cancelLeftoverActiveRuns(cmd.agentId, cmd.cwd);
    return await agent.send(cmd.prompt, { mcpServers });
  }
}

async function cancelActiveRun(): Promise<void> {
  const run = state.activeRun;
  state.activeRun = null;
  if (!run || !isActiveRunStatus(run.status)) {
    return;
  }
  try {
    await withTimeout(Promise.resolve(run.cancel()), CANCEL_TIMEOUT_MS, "run.cancel timed out");
  } catch {
    // Execute still emits the terminal result after the abort gate trips.
  }
}

export async function handleExecute(cmd: ExecuteCommand, emit: EmitFn): Promise<void> {
  let agent: SDKAgent | null = null;
  let settled = false;
  try {
    const apiKey = resolveApiKey(cmd.apiKeyEnv);
    if (!apiKey) {
      emit({ event: "error", message: "missing CURSOR_API_KEY", retryable: false });
      return;
    }

    const options = buildAgentOptions(cmd, apiKey);
    const mcpServers = options.mcpServers;

    agent = cmd.agentId
      ? await Agent.resume(cmd.agentId, options)
      : await Agent.create(options);

    state.activeAgent = agent;
    emit({ event: "agent_id", agentId: agent.agentId });

    if (cmd.agentId) {
      await cancelLeftoverActiveRuns(agent.agentId, cmd.cwd);
    }

    const run = await sendPrompt(agent, cmd, mcpServers);
    state.activeRun = run;

    const abortGate = createAbortGate();
    activeExecuteControl = { abort: abortGate.abort };

    await consumeRunStream(run, emit, abortGate.signal);
    const cancelled = abortGate.signal.aborted;

    const result = await waitForRunResult(run, cancelled);
    const rawStatus = String(result.status ?? "");
    const status =
      rawStatus === "finished"
        ? "completed"
        : rawStatus === "canceled" || rawStatus === "cancelled" || cancelled
          ? "cancelled"
          : rawStatus;
    emit({
      event: "result",
      status,
      output: result.result ?? "",
      usage: result.usage,
      error: result.error?.message,
    });
    settled = true;
  } catch (err) {
    emit({
      event: "error",
      message: err instanceof Error ? err.message : String(err),
      retryable: false,
    });
  } finally {
    activeExecuteControl = null;
    if (!settled) {
      await cancelActiveRun();
    } else {
      state.activeRun = null;
    }
    state.activeAgent = null;
    await disposeAgent(agent);
  }
}

export async function handleSteer(cmd: SteerCommand, emit: EmitFn): Promise<void> {
  const run = state.activeRun;
  if (!run?.steer) {
    emit({ event: "error", message: "no active run to steer", retryable: false });
    return;
  }
  try {
    await run.steer(cmd.text);
  } catch (err) {
    emit({
      event: "error",
      message: err instanceof Error ? err.message : String(err),
      retryable: false,
    });
  }
}

export async function handleCancel(cmd: CancelCommand, emit: EmitFn): Promise<void> {
  const run = state.activeRun;
  if (!run) {
    emit({ event: "error", message: "no active run to cancel", retryable: false });
    return;
  }
  // Abort the execute stream first. run.cancel() can wait for the stream
  // consumer, so awaiting it here deadlocks a hung local run.
  activeExecuteControl?.abort();
  try {
    await withTimeout(Promise.resolve(run.cancel()), CANCEL_TIMEOUT_MS, "run.cancel timed out");
  } catch {
    // Execute still emits the terminal result after the abort gate trips.
  }
}

export async function handleReload(cmd: ReloadCommand, emit: EmitFn): Promise<void> {
  const agent = state.activeAgent;
  if (!agent) {
    emit({ event: "error", message: "no active agent to reload", retryable: false });
    return;
  }
  try {
    await agent.reload();
  } catch (err) {
    emit({
      event: "error",
      message: err instanceof Error ? err.message : String(err),
      retryable: false,
    });
  }
}

export async function handleMessagesList(cmd: MessagesListCommand, emit: EmitFn): Promise<void> {
  try {
    const apiKey = resolveApiKey(cmd.apiKeyEnv);
    if (!apiKey) {
      emit({ event: "error", message: "missing CURSOR_API_KEY", retryable: false });
      return;
    }
    const messages = await Agent.messages.list(cmd.agentId, {
      runtime: "local",
      cwd: cmd.cwd,
    });
    emit({ event: "messages", items: messages });
  } catch (err) {
    emit({
      event: "error",
      message: err instanceof Error ? err.message : String(err),
      retryable: false,
    });
  }
}

export async function handleListModels(cmd: ListModelsCommand, emit: EmitFn): Promise<void> {
  try {
    const apiKey = resolveApiKey(cmd.apiKeyEnv);
    if (!apiKey) {
      emit({ event: "error", message: "missing CURSOR_API_KEY", retryable: false });
      return;
    }
    const items = await Cursor.models.list({ apiKey });
    emit({ event: "models", items });
  } catch (err) {
    emit({
      event: "error",
      message: err instanceof Error ? err.message : String(err),
      retryable: false,
    });
  }
}

export async function handleShutdown(emit: EmitFn): Promise<void> {
  activeExecuteControl?.abort();
  await cancelActiveRun();
  const agent = state.activeAgent;
  state.activeAgent = null;
  await disposeAgent(agent);
}

export async function dispatchCommand(cmd: IpcCommand, emit: EmitFn): Promise<void> {
  switch (cmd.cmd) {
    case "execute":
      await handleExecute(cmd, emit);
      break;
    case "steer":
      await handleSteer(cmd, emit);
      break;
    case "cancel":
      await handleCancel(cmd, emit);
      break;
    case "reload":
      await handleReload(cmd, emit);
      break;
    case "list-models":
      await handleListModels(cmd, emit);
      break;
    case "messages-list":
      await handleMessagesList(cmd, emit);
      break;
    case "shutdown":
      await handleShutdown(emit);
      break;
    default: {
      const unknownCmd = (cmd as { cmd?: string }).cmd ?? "unknown";
      emit({
        event: "error",
        message: `unknown command: ${unknownCmd}`,
        retryable: false,
      });
    }
  }
}
