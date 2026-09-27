"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { createWorkspaceAwareStorage, registerForWorkspaceRehydration } from "../../platform/workspace-storage";
import { defaultStorage } from "../../platform/storage";

/** Default Generate-debug kickoff; matches `comment.debug.kickoff_message` in en. */
export const DEFAULT_DEBUG_KICKOFF_MESSAGE =
  "Start a debug session. Write 3–5 hypotheses, add probes, then tell me how to retest. Always push your work to the branch so I can retest from my main local checkout, not your worktree.";

interface DebugSessionSettingsState {
  /** Empty means use the locale default. The agent mention is prepended at send time. */
  kickoffMessage: string;
  setKickoffMessage: (message: string) => void;
}

export function resolveDebugKickoffMessage(
  stored: string,
  fallback: string = DEFAULT_DEBUG_KICKOFF_MESSAGE,
): string {
  const trimmed = stored.trim();
  return trimmed.length > 0 ? trimmed : fallback;
}

/** Persist empty when the field matches the locale default so language switches still apply. */
export function persistDebugKickoffMessage(next: string, fallback: string): string {
  const trimmed = next.trim();
  if (trimmed.length === 0 || trimmed === fallback.trim()) return "";
  return next;
}

export function buildDebugKickoffContent(
  agentName: string,
  agentId: string,
  storedMessage: string,
  fallback: string = DEFAULT_DEBUG_KICKOFF_MESSAGE,
): string {
  const name = agentName.trim() || "agent";
  return `[@${name}](mention://agent/${agentId}) ${resolveDebugKickoffMessage(storedMessage, fallback)}`;
}

export const useDebugSessionSettingsStore = create<DebugSessionSettingsState>()(
  persist(
    (set) => ({
      kickoffMessage: "",
      setKickoffMessage: (kickoffMessage) => set({ kickoffMessage }),
    }),
    {
      name: "multica_debug_session_settings",
      storage: createJSONStorage(() => createWorkspaceAwareStorage(defaultStorage)),
      merge: (persistedState, currentState) => {
        const persisted = (persistedState ?? {}) as Partial<DebugSessionSettingsState>;
        return {
          ...currentState,
          ...persisted,
          kickoffMessage: persisted.kickoffMessage ?? "",
        };
      },
    },
  ),
);

registerForWorkspaceRehydration(() => useDebugSessionSettingsStore.persist.rehydrate());
