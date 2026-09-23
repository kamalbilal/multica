import { beforeEach, describe, expect, it, vi } from "vitest";
import type { SDKAgent, Run } from "@cursor/sdk";

const mockDispose = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
const mockSend = vi.fn<SDKAgent["send"]>();
const mockCreate = vi.fn<(options: unknown) => Promise<SDKAgent>>();
const mockResume = vi.fn<(agentId: string, options?: unknown) => Promise<SDKAgent>>();
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

vi.mock("@cursor/sdk", () => ({
  Agent: {
    create: (...args: unknown[]) => mockCreate(...args),
    resume: (...args: unknown[]) => mockResume(...args),
    messages: {
      list: (...args: unknown[]) => mockMessagesList(...args),
    },
  },
  Cursor: {
    models: {
      list: (...args: unknown[]) => mockModelsList(...args),
    },
  },
}));

import { dispatchCommand, handleExecute, handleSteer } from "./executor.js";

describe("executor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env.CURSOR_API_KEY = "test-api-key";
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
