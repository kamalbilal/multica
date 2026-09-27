import { beforeEach, describe, expect, it } from "vitest";
import {
  DEFAULT_DEBUG_KICKOFF_MESSAGE,
  buildDebugKickoffContent,
  persistDebugKickoffMessage,
  resolveDebugKickoffMessage,
  useDebugSessionSettingsStore,
} from "./debug-session-settings-store";

describe("debug session kickoff message", () => {
  beforeEach(() => {
    useDebugSessionSettingsStore.setState({ kickoffMessage: "" });
  });

  it("uses the locale fallback when nothing is stored", () => {
    expect(resolveDebugKickoffMessage("", "Locale default")).toBe("Locale default");
    expect(resolveDebugKickoffMessage("   ")).toBe(DEFAULT_DEBUG_KICKOFF_MESSAGE);
  });

  it("keeps a custom stored message", () => {
    expect(resolveDebugKickoffMessage("  Check the login form.  ", "fallback")).toBe(
      "Check the login form.",
    );
  });

  it("does not persist text that matches the locale default", () => {
    expect(persistDebugKickoffMessage(DEFAULT_DEBUG_KICKOFF_MESSAGE, DEFAULT_DEBUG_KICKOFF_MESSAGE)).toBe(
      "",
    );
    expect(persistDebugKickoffMessage("   ", DEFAULT_DEBUG_KICKOFF_MESSAGE)).toBe("");
    expect(persistDebugKickoffMessage("  Check logs.  ", DEFAULT_DEBUG_KICKOFF_MESSAGE)).toBe(
      "  Check logs.  ",
    );
  });

  it("prepends the agent mention and never drops it", () => {
    expect(
      buildDebugKickoffContent("Codex", "agent-1", "Look at the 500s.", "fallback"),
    ).toBe("[@Codex](mention://agent/agent-1) Look at the 500s.");
    expect(buildDebugKickoffContent("", "agent-1", "", "fallback")).toBe(
      "[@agent](mention://agent/agent-1) fallback",
    );
  });

  it("stores a custom kickoff on the workspace-aware store", () => {
    useDebugSessionSettingsStore.getState().setKickoffMessage("Custom kickoff");
    expect(useDebugSessionSettingsStore.getState().kickoffMessage).toBe("Custom kickoff");
  });
});
