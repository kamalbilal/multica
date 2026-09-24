// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { AgentTask } from "@multica/core/types";
import { selectRunningTurnGuidanceTask, tasksFromCommentRuns } from "./running-turn-guidance";

function runningTask(overrides: Partial<AgentTask> = {}): AgentTask {
  return {
    id: "task-1",
    issue_id: "issue-1",
    agent_id: "agent-1",
    status: "running",
    supplement_capability: "task-supplement-v1",
    can_supplement: true,
    created_at: "2026-09-24T00:00:00Z",
    started_at: "2026-09-24T00:01:00Z",
    ...overrides,
  } as AgentTask;
}

describe("selectRunningTurnGuidanceTask", () => {
  it("picks the latest started running task even without supplement capability", () => {
    const older = runningTask({ id: "older", started_at: "2026-09-24T00:00:00Z" });
    const newer = runningTask({
      id: "newer",
      started_at: "2026-09-24T00:05:00Z",
      supplement_capability: undefined,
      can_supplement: undefined,
    });
    expect(selectRunningTurnGuidanceTask([older, newer])?.id).toBe("newer");
  });

  it("ignores completed tasks", () => {
    expect(selectRunningTurnGuidanceTask([
      runningTask({ status: "completed", supplement_capability: "task-supplement-v1" }),
    ])).toBeUndefined();
  });

  it("uses the visible thread run when the issue task list is empty", () => {
    const visible = runningTask({ id: "visible-run", supplement_capability: undefined });
    expect(selectRunningTurnGuidanceTask(tasksFromCommentRuns(undefined, [visible]))?.id).toBe("visible-run");
  });
});
