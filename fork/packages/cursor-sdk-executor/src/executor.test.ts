import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SDKAgent, Run } from "@cursor/sdk";

const mockDispose = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
const mockSend = vi.fn<SDKAgent["send"]>();
const mockCreate = vi.fn<(options: unknown) => Promise<SDKAgent>>();
const mockResume = vi.fn<(agentId: string, options?: unknown) => Promise<SDKAgent>>();
const mockListRuns = vi.fn<(agentId: string, options?: unknown) => Promise<{ items: Run[] }>>();
const mockCancelRun = vi.fn<(runId: string, options?: unknown) => Promise<void>>();
const mockModelsList = vi.fn<(options?: { apiKey?: string }) => Promise<unknown[]>>();
const mockMessagesList = vi.fn<
  (agentId: string, options?: { runtime?: string; cwd?: string }) => Promise<unknown[]>
>();

function createMockAgent(agentId = "agent-test-1"): SDKAgent {
  return {
    agentId,
    model: undefined,
    send: mockSend,
    close: vi.fn(),
    reload: vi.fn().mockResolvedValue(undefined),
    [Symbol.asyncDispose]: mockDispose,
    listArtifacts: vi.fn().mockResolvedValue([]),
    downloadArtifact: vi.fn().mockResolvedValue(Buffer.from("")),
    getUsage: vi.fn(),
  };
}

function createMockRun(messages: unknown[] = []): Run {
  return {
    id: "run-test-1",
    agentId: "agent-test-1",
    supports: vi.fn().mockReturnValue(true),
    unsupportedReason: vi.fn().mockReturnValue(undefined),
    stream: async function* () {
      for (const message of messages) {
        yield message as never;
      }
    },
    conversation: vi.fn().mockResolvedValue([]),
    wait: vi.fn().mockResolvedValue({
      id: "run-test-1",
      status: "finished",
      result: "done",
    }),
    cancel: vi.fn().mockResolvedValue(undefined),
    steer: vi.fn().mockResolvedValue("complete_delivered"),
    status: "running",
    onDidChangeStatus: vi.fn().mockReturnValue(() => {}),
  };
}

vi.mock("@cursor/sdk", () => {
  class AgentBusyError extends Error {
    constructor(message = "Agent already has active run") {
      super(message);
      this.name = "AgentBusyError";
    }
  }
  return {
    Agent: {
      create: (...args: unknown[]) => mockCreate(...args),
      resume: (...args: unknown[]) => mockResume(...args),
      listRuns: (...args: unknown[]) => mockListRuns(...args),
      cancelRun: (...args: unknown[]) => mockCancelRun(...args),
      messages: {
        list: (...args: unknown[]) => mockMessagesList(...args),
      },
    },
    AgentBusyError,
    Cursor: {
      models: {
        list: (...args: unknown[]) => mockModelsList(...args),
      },
    },
  };
});

import { AgentBusyError } from "@cursor/sdk";
import { dispatchCommand, handleCancel, handleExecute, handleSteer } from "./executor.js";

describe("executor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env.CURSOR_API_KEY = "test-api-key";
    mockListRuns.mockResolvedValue({ items: [] });
    mockCancelRun.mockResolvedValue(undefined);
  });

  it("execute emits agent_id, message, and result", async () => {
    const assistantMessage = {
      type: "assistant",
      agent_id: "agent-test-1",
      run_id: "run-test-1",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "hello" }],
      },
    };
    const mockRun = createMockRun([assistantMessage]);
    const mockAgent = createMockAgent();
    mockSend.mockResolvedValue(mockRun);
    mockCreate.mockResolvedValue(mockAgent);

    const events: unknown[] = [];
    await handleExecute(
      {
        cmd: "execute",
        id: "req-1",
        prompt: "say hello",
        cwd: "/tmp/workdir",
        model: "composer-2.5",
      },
      (event) => events.push(event),
    );

    expect(mockCreate).toHaveBeenCalledWith({
      apiKey: "test-api-key",
      model: { id: "composer-2.5" },
      local: {
        cwd: "/tmp/workdir",
        settingSources: ["project"],
      },
      mcpServers: undefined,
    });
    expect(mockDispose).toHaveBeenCalledOnce();
    expect(events).toEqual([
      { event: "agent_id", agentId: "agent-test-1" },
      { event: "message", type: "assistant", content: "hello" },
      {
        event: "result",
        status: "completed",
        output: "done",
        usage: undefined,
        error: undefined,
      },
    ]);
  });

  it("execute resumes when agentId is provided", async () => {
    const mockRun = createMockRun();
    const mockAgent = createMockAgent("agent-resumed");
    mockSend.mockResolvedValue(mockRun);
    mockResume.mockResolvedValue(mockAgent);

    const events: unknown[] = [];
    await handleExecute(
      {
        cmd: "execute",
        id: "req-2",
        prompt: "continue",
        cwd: "/tmp/workdir",
        agentId: "agent-resumed",
        model: "composer-2.5",
      },
      (event) => events.push(event),
    );

    expect(mockResume).toHaveBeenCalledWith("agent-resumed", {
      apiKey: "test-api-key",
      model: { id: "composer-2.5" },
      local: {
        cwd: "/tmp/workdir",
        settingSources: ["project"],
      },
    });
    expect(mockCreate).not.toHaveBeenCalled();
    expect(events[0]).toEqual({ event: "agent_id", agentId: "agent-resumed" });
    expect(mockListRuns).toHaveBeenCalledWith("agent-resumed", {
      runtime: "local",
      cwd: "/tmp/workdir",
      limit: 20,
    });
  });

  it("execute cancels leftover active runs before send on resume", async () => {
    const leftover = createMockRun();
    leftover.id = "run-leftover";
    leftover.status = "running";
    leftover.cancel = vi.fn().mockResolvedValue(undefined);
    mockListRuns.mockResolvedValue({ items: [leftover] });

    const mockRun = createMockRun();
    const mockAgent = createMockAgent("agent-resumed");
    mockSend.mockResolvedValue(mockRun);
    mockResume.mockResolvedValue(mockAgent);

    await handleExecute(
      {
        cmd: "execute",
        id: "req-cancel-leftover",
        prompt: "continue",
        cwd: "/tmp/workdir",
        agentId: "agent-resumed",
        model: "composer-2.5",
      },
      () => {},
    );

    expect(leftover.cancel).toHaveBeenCalledOnce();
    expect(mockSend).toHaveBeenCalledOnce();
  });

  it("execute retries send after AgentBusyError", async () => {
    const leftover = createMockRun();
    leftover.id = "run-leftover";
    leftover.status = "running";
    leftover.supports = vi.fn().mockReturnValue(false);
    mockListRuns
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({ items: [leftover] });
    mockCancelRun.mockResolvedValue(undefined);

    const mockRun = createMockRun();
    const mockAgent = createMockAgent("agent-resumed");
    mockSend
      .mockRejectedValueOnce(new AgentBusyError("Agent agent-resumed already has active run"))
      .mockResolvedValueOnce(mockRun);
    mockResume.mockResolvedValue(mockAgent);

    const events: unknown[] = [];
    await handleExecute(
      {
        cmd: "execute",
        id: "req-busy-retry",
        prompt: "continue",
        cwd: "/tmp/workdir",
        agentId: "agent-resumed",
        model: "composer-2.5",
      },
      (event) => events.push(event),
    );

    expect(mockSend).toHaveBeenCalledTimes(2);
    expect(mockCancelRun).toHaveBeenCalledWith("run-leftover", {
      runtime: "local",
      cwd: "/tmp/workdir",
    });
    expect(events.at(-1)).toMatchObject({ event: "result", status: "completed" });
  });

  it("execute emits error when CURSOR_API_KEY is missing", async () => {
    delete process.env.CURSOR_API_KEY;

    const events: unknown[] = [];
    await handleExecute(
      {
        cmd: "execute",
        id: "req-3",
        prompt: "test",
        cwd: "/tmp/workdir",
        model: "composer-2.5",
      },
      (event) => events.push(event),
    );

    expect(events).toEqual([
      { event: "error", message: "missing CURSOR_API_KEY", retryable: false },
    ]);
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("execute stops streaming after terminal status", async () => {
    const mockRun = createMockRun([
      { type: "status", status: "Running" },
      { type: "status", status: "Finished" },
      { type: "assistant", agent_id: "agent-test-1", run_id: "run-test-1", message: { role: "assistant", content: [{ type: "text", text: "late" }] } },
    ]);
    mockSend.mockResolvedValue(mockRun);
    mockCreate.mockResolvedValue(createMockAgent());

    const events: unknown[] = [];
    await handleExecute(
      {
        cmd: "execute",
        id: "req-terminal-status",
        prompt: "say hello",
        cwd: "/tmp/workdir",
        model: "composer-2.5",
      },
      (event) => events.push(event),
    );

    expect(events).toEqual([
      { event: "agent_id", agentId: "agent-test-1" },
      { event: "message", type: "status", status: "running" },
      { event: "message", type: "status", status: "finished" },
      {
        event: "message",
        type: "assistant",
        content: "late",
      },
      {
        event: "result",
        status: "completed",
        output: "done",
        usage: undefined,
        error: undefined,
      },
    ]);
  });

  it("steer targets the active run", async () => {
    const mockRun = createMockRun();
    mockRun.stream = async function* () {
      await new Promise(() => {});
    };
    mockRun.wait = vi.fn(() => new Promise(() => {}));
    mockSend.mockResolvedValue(mockRun);
    mockCreate.mockResolvedValue(createMockAgent());

    void handleExecute(
      {
        cmd: "execute",
        id: "req-steer-setup",
        prompt: "start",
        cwd: "/tmp/workdir",
        model: "composer-2.5",
      },
      () => {},
    );

    for (let attempt = 0; attempt < 50 && mockSend.mock.calls.length === 0; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 10));
    }
    expect(mockSend).toHaveBeenCalledOnce();

    await handleSteer({ cmd: "steer", id: "req-steer", text: "add this detail" }, () => {});

    expect(mockRun.steer).toHaveBeenCalledWith("add this detail");
  });

  it("cancel unblocks execute when stream and run.cancel hang", async () => {
    const mockRun = createMockRun();
    mockRun.stream = async function* () {
      yield { type: "status", status: "Running" } as never;
      await new Promise(() => {});
    };
    mockRun.wait = vi.fn(() => new Promise(() => {}));
    mockRun.cancel = vi.fn(() => new Promise(() => {}));
    mockSend.mockResolvedValue(mockRun);
    mockCreate.mockResolvedValue(createMockAgent());

    const events: unknown[] = [];
    const executePromise = handleExecute(
      {
        cmd: "execute",
        id: "req-cancel-deadlock",
        prompt: "start",
        cwd: "/tmp/workdir",
        model: "composer-2.5",
      },
      (event) => events.push(event),
    );

    for (let attempt = 0; attempt < 50 && !events.some((event) => JSON.stringify(event).includes("running")); attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 10));
    }
    expect(events).toContainEqual({ event: "message", type: "status", status: "running" });

    await handleCancel({ cmd: "cancel", id: "req-cancel" }, () => {});

    await Promise.race([
      executePromise,
      new Promise((_, reject) => {
        setTimeout(() => reject(new Error("execute did not finish after cancel")), 4000);
      }),
    ]);

    expect(events).toContainEqual({ event: "message", type: "status", status: "running" });
    expect(events.at(-1)).toEqual({
      event: "result",
      status: "cancelled",
      output: "",
      usage: undefined,
      error: undefined,
    });
  });

  it("list-models emits models event", async () => {
    mockModelsList.mockResolvedValue([{ id: "composer-2.5" }]);

    const events: unknown[] = [];
    await dispatchCommand(
      { cmd: "list-models", id: "req-4" },
      (event) => events.push(event),
    );

    expect(mockModelsList).toHaveBeenCalledWith({ apiKey: "test-api-key" });
    expect(events).toEqual([{ event: "models", items: [{ id: "composer-2.5" }] }]);
  });

  it("messages-list emits messages event", async () => {
    mockMessagesList.mockResolvedValue([
      { type: "assistant", message: { role: "assistant", content: [{ type: "text", text: "hello" }] } },
    ]);

    const events: unknown[] = [];
    await dispatchCommand(
      {
        cmd: "messages-list",
        id: "req-5",
        agentId: "agent-test-1",
        cwd: "/tmp/workdir",
      },
      (event) => events.push(event),
    );

    expect(mockMessagesList).toHaveBeenCalledWith("agent-test-1", {
      runtime: "local",
      cwd: "/tmp/workdir",
    });
    expect(events).toEqual([
      {
        event: "messages",
        items: [
          {
            type: "assistant",
            message: { role: "assistant", content: [{ type: "text", text: "hello" }] },
          },
        ],
      },
    ]);
  });
});
