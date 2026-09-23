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

export type IpcCommand =
  | ExecuteCommand
  | SteerCommand
  | CancelCommand
  | ReloadCommand
  | ShutdownCommand;

export type IpcMessageEvent =
  | { event: "message"; type: "assistant"; content: string }
  | { event: "message"; type: "thinking"; content: string }
  | { event: "message"; type: "tool_use"; tool: string; callId: string; input: unknown }
  | { event: "message"; type: "tool_result"; tool: string; callId: string; output: string }
  | { event: "message"; type: "status"; status: string }
  | { event: "message"; type: "usage"; usage: unknown };

/** Node → Go: terminal run outcome. */
export interface IpcResultEvent {
  event: "result";
  status: "completed" | "failed" | "cancelled" | string;
  output?: string;
  usage?: unknown;
  error?: string;
}
