import type { SDKMessage } from "@cursor/sdk";
import type { IpcMessageEvent } from "./protocol.js";

function serializeToolOutput(result: unknown): string {
  if (result === undefined || result === null) {
    return "";
  }
  if (typeof result === "string") {
    return result;
  }
  return JSON.stringify(result);
}

export function mapSdkMessage(event: SDKMessage): IpcMessageEvent | null {
  switch (event.type) {
    case "assistant": {
      const content = event.message.content
        .filter((block) => block.type === "text")
        .map((block) => block.text)
        .join("");
      if (!content) {
        return null;
      }
      return { event: "message", type: "assistant", content };
    }
    case "thinking":
      return { event: "message", type: "thinking", content: event.text };
    case "tool_call": {
      if (event.status === "running") {
        return {
          event: "message",
          type: "tool_use",
          tool: event.name,
          callId: event.call_id,
          input: event.args ?? {},
        };
      }
      if (event.status === "completed" || event.status === "error") {
        return {
          event: "message",
          type: "tool_result",
          tool: event.name,
          callId: event.call_id,
          output: serializeToolOutput(event.result),
        };
      }
      return null;
    }
    case "status":
      return {
        event: "message",
        type: "status",
        status: event.status.toLowerCase(),
      };
    case "usage":
      return {
        event: "message",
        type: "usage",
        usage: event.usage,
      };
    default:
      return null;
  }
}
