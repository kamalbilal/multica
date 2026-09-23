import {
  Agent,
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

export async function handleExecute(cmd: ExecuteCommand, emit: EmitFn): Promise<void> {
  let agent: SDKAgent | null = null;
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

    const run = await agent.send(cmd.prompt, { mcpServers });
    state.activeRun = run;

    for await (const sdkEvent of run.stream()) {
      const mapped = mapSdkMessage(sdkEvent);
      if (mapped) {
        emit(mapped);
      }
    }

    const result = await run.wait();
    emit({
      event: "result",
      status: result.status === "finished" ? "completed" : result.status,
      output: result.result ?? "",
      usage: result.usage,
      error: result.error?.message,
    });
  } catch (err) {
    emit({
      event: "error",
      message: err instanceof Error ? err.message : String(err),
      retryable: false,
    });
  } finally {
    state.activeRun = null;
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
  try {
    await run.cancel();
  } catch (err) {
    emit({
      event: "error",
      message: err instanceof Error ? err.message : String(err),
      retryable: false,
    });
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
  state.activeRun = null;
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
