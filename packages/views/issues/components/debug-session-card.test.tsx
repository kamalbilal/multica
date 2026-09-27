"use client";

import { fireEvent, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DebugSessionCard } from "./debug-session-card";
import { renderWithI18n } from "../../test/i18n";

const getIssueDebugSession = vi.fn();
const continueIssueDebugSession = vi.fn();
const closeIssueDebugSession = vi.fn();

vi.mock("@multica/core/api", () => ({
  api: {
    getIssueDebugSession: (...args: unknown[]) => getIssueDebugSession(...args),
    continueIssueDebugSession: (...args: unknown[]) => continueIssueDebugSession(...args),
    closeIssueDebugSession: (...args: unknown[]) => closeIssueDebugSession(...args),
  },
}));

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      <DebugSessionCard issueId="issue-1" />
    </QueryClientProvider>,
  );
}

describe("DebugSessionCard", () => {
  beforeEach(() => {
    getIssueDebugSession.mockReset();
    continueIssueDebugSession.mockReset();
    closeIssueDebugSession.mockReset();
  });

  it("shows Approved and Looks fixed while waiting for a repro", async () => {
    getIssueDebugSession.mockResolvedValue({
      session: {
        id: "sess-1",
        issue_id: "issue-1",
        agent_id: "agent-1",
        status: "waiting_repro",
        hypotheses: [{ id: "H1", text: "token is empty" }],
        repro_steps: "Open the login page and submit.",
      },
    });
    renderCard();
    expect(await screen.findByText("Open the login page and submit.")).toBeInTheDocument();
    expect(screen.getByText("0 logs captured")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Approved" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Looks fixed" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeInTheDocument();
  });

  it("shows the work-dir hint while waiting", async () => {
    getIssueDebugSession.mockResolvedValue({
      session: {
        id: "sess-1",
        issue_id: "issue-1",
        agent_id: "agent-1",
        status: "waiting_repro",
        hypotheses: [],
        repro_steps: "Open the login page.",
        work_dir_hint: "/tmp/worktree",
      },
    });
    renderCard();
    expect(await screen.findByText("Retest in /tmp/worktree")).toBeInTheDocument();
  });

  it("closes the session from Dismiss", async () => {
    getIssueDebugSession.mockResolvedValue({
      session: {
        id: "sess-1",
        issue_id: "issue-1",
        agent_id: "agent-1",
        status: "waiting_repro",
        hypotheses: [],
        repro_steps: "Open the login page.",
      },
    });
    closeIssueDebugSession.mockResolvedValue({ session: { status: "closed" } });
    renderCard();
    fireEvent.click(await screen.findByRole("button", { name: "Dismiss" }));
    await vi.waitFor(() => {
      expect(closeIssueDebugSession).toHaveBeenCalledWith("issue-1");
    });
  });

  it("hides when there is no waiting session", async () => {
    getIssueDebugSession.mockResolvedValue({ session: null });
    const { container } = renderCard();
    await vi.waitFor(() => {
      expect(getIssueDebugSession).toHaveBeenCalled();
    });
    expect(screen.queryByRole("button", { name: "Approved" })).not.toBeInTheDocument();
    expect(container.querySelector("[class*='ring-border']")).toBeNull();
  });

  it("hides retest details when the header is collapsed", async () => {
    getIssueDebugSession.mockResolvedValue({
      session: {
        id: "sess-1",
        issue_id: "issue-1",
        agent_id: "agent-1",
        status: "waiting_repro",
        hypotheses: [{ id: "H1", text: "token is empty" }],
        repro_steps: "Open the login page and submit.",
        work_dir_hint: "/tmp/worktree",
      },
    });
    renderCard();
    expect(await screen.findByText("Open the login page and submit.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Hide details" }));
    expect(screen.queryByText("Open the login page and submit.")).not.toBeInTheDocument();
    expect(screen.queryByText("Retest in /tmp/worktree")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Approved" })).toBeInTheDocument();
    expect(screen.getByText("0 logs captured")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show details" }));
    expect(screen.getByText("Open the login page and submit.")).toBeInTheDocument();
  });

  it("shows the captured log count while waiting", async () => {
    getIssueDebugSession.mockResolvedValue({
      session: {
        id: "sess-1",
        issue_id: "issue-1",
        agent_id: "agent-1",
        status: "waiting_repro",
        hypotheses: [{ id: "H1", text: "token is empty" }],
        repro_steps: "Open the login page and submit.",
        event_count: 4,
      },
    });
    renderCard();
    expect(await screen.findByText("4 logs captured")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Hide details" }));
    expect(screen.getByText("4 logs captured")).toBeInTheDocument();
    expect(screen.queryByText("Open the login page and submit.")).not.toBeInTheDocument();
  });
});
