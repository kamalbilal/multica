import type { AgentTask } from "@multica/core/types";

/** Latest in-flight issue task that can take mid-run guidance. */
export function selectRunningTurnGuidanceTask(
  tasks: readonly AgentTask[] | undefined,
): AgentTask | undefined {
  if (!tasks?.length) return undefined;
  const candidates = tasks.filter((task) => task.status === "running");
  if (candidates.length === 0) return undefined;
  return candidates.reduce((latest, task) => {
    const latestAt = Date.parse(latest.started_at ?? latest.created_at);
    const taskAt = Date.parse(task.started_at ?? task.created_at);
    return taskAt > latestAt ? task : latest;
  });
}

export function canSendRunningTurnGuidance(task: AgentTask): boolean {
  if (task.status !== "running") return false;
  if (task.can_supplement === false) return false;
  return true;
}

export function tasksFromCommentRuns(
  issueTasks: readonly AgentTask[] | undefined,
  runTasks: readonly AgentTask[] | undefined,
): AgentTask[] {
  const merged = new Map<string, AgentTask>();
  for (const task of issueTasks ?? []) merged.set(task.id, task);
  for (const task of runTasks ?? []) {
    const current = merged.get(task.id);
    merged.set(task.id, current ? { ...current, ...task } : task);
  }
  return [...merged.values()];
}
