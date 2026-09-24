/** Go → Node: start or resume an agent run. */
export interface ExecuteCommand {
  cmd: "execute";
  id: string;
  prompt: string;
  cwd: string;
  agentId?: string | null;
  model: string;
  mcpConfig?: Record<string, unknown>;
  customTools?: Record<string, unknown>;
  sandboxOptions?: Record<string, unknown>;
  apiKeyEnv?: string;
}

/** Go → Node: steer the active run with supplemental text. */
export interface SteerCommand {
  cmd: "steer";
  id: string;
  text: string;
}

/** Go → Node: cancel the active run. */
export interface CancelCommand {
  cmd: "cancel";
  id: string;
}

/** Go → Node: reload agent configuration after daemon MCP refresh. */
export interface ReloadCommand {
  cmd: "reload";
  id: string;
}

/** Go → Node: shut down the executor process. */
export interface ShutdownCommand {
  cmd: "shutdown";
  id: string;
}

/** Go → Node: list models available to the authenticated API key. */
export interface ListModelsCommand {
  cmd: "list-models";
  id: string;
  apiKeyEnv?: string;
}

/** Go → Node: list stored messages for a local agent (resume backfill). */
export interface MessagesListCommand {
  cmd: "messages-list";
  id: string;
  agentId: string;
  cwd: string;
  apiKeyEnv?: string;
}

export type IpcCommand =
  | ExecuteCommand
  | SteerCommand
  | CancelCommand
  | ReloadCommand
  | ShutdownCommand
  | ListModelsCommand
  | MessagesListCommand;

export type IpcMessageEvent =
  | { event: "message"; type: "assistant"; content: string }
  | { event: "message"; type: "thinking"; content: string }
  | { event: "message"; type: "tool_use"; tool: string; callId: string; input: unknown }
  | { event: "message"; type: "tool_result"; tool: string; callId: string; output: string; status?: string }
  | { event: "message"; type: "status"; status: string }
  | { event: "message"; type: "usage"; usage: unknown };

/** Node → Go: SDK agent id after create or resume. */
export interface IpcAgentIdEvent {
  event: "agent_id";
  agentId: string;
}

/** Node → Go: model catalog from list-models. */
export interface IpcModelsEvent {
  event: "models";
  items: unknown[];
}

/** Node → Go: stored transcript from messages-list. */
export interface IpcMessagesEvent {
  event: "messages";
  items: unknown[];
}

/** Node → Go: non-terminal executor error. */
export interface IpcErrorEvent {
  event: "error";
  message: string;
  retryable: boolean;
}

/** Node → Go: terminal run outcome. */
export interface IpcResultEvent {
  event: "result";
  status: "completed" | "failed" | "cancelled" | string;
  output?: string;
  usage?: unknown;
  error?: string;
}

export type IpcEvent =
  | IpcAgentIdEvent
  | IpcMessageEvent
  | IpcModelsEvent
  | IpcMessagesEvent
  | IpcErrorEvent
  | IpcResultEvent;
