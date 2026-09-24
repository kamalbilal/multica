import { describe, expect, it } from "vitest";
import { mapSdkMessage } from "./stream-mapper.js";

describe("mapSdkMessage", () => {
  it("maps assistant text blocks", () => {
    const out = mapSdkMessage({
      type: "assistant",
      agent_id: "agent-1",
      run_id: "run-1",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "hello" }],
      },
    });
    expect(out).toEqual({
      event: "message",
      type: "assistant",
      content: "hello",
    });
  });

  it("maps thinking text", () => {
    const out = mapSdkMessage({
      type: "thinking",
      agent_id: "agent-1",
      run_id: "run-1",
      text: "reasoning here",
    });
    expect(out).toEqual({
      event: "message",
      type: "thinking",
      content: "reasoning here",
    });
  });

  it("maps tool_call started", () => {
    const out = mapSdkMessage({
      type: "tool_call",
      agent_id: "agent-1",
      run_id: "run-1",
      name: "grep",
      status: "running",
      call_id: "c1",
      args: { pattern: "foo" },
    });
    expect(out).toEqual({
      event: "message",
      type: "tool_use",
      tool: "grep",
      callId: "c1",
      input: { pattern: "foo" },
    });
  });

  it("maps tool_call completed", () => {
    const out = mapSdkMessage({
      type: "tool_call",
      agent_id: "agent-1",
      run_id: "run-1",
      name: "grep",
      status: "completed",
      call_id: "c1",
      args: { pattern: "foo" },
      result: { matches: 1 },
    });
    expect(out?.type).toBe("tool_result");
    expect(out?.tool).toBe("grep");
    expect(out?.callId).toBe("c1");
    expect(out?.status).toBe("completed");
  });

  it("maps status", () => {
    const out = mapSdkMessage({
      type: "status",
      agent_id: "agent-1",
      run_id: "run-1",
      status: "RUNNING",
    });
    expect(out).toEqual({
      event: "message",
      type: "status",
      status: "running",
    });
  });

  it("maps usage passthrough", () => {
    const usage = {
      inputTokens: 10,
      outputTokens: 5,
      totalTokens: 15,
    };
    const out = mapSdkMessage({
      type: "usage",
      agent_id: "agent-1",
      run_id: "run-1",
      usage,
    });
    expect(out).toEqual({
      event: "message",
      type: "usage",
      usage,
    });
  });
});
