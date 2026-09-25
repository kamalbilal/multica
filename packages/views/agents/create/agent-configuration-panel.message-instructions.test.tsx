import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen } from "@testing-library/react";
import { EMPTY_AGENT_DRAFT } from "@multica/core/agents";
import { renderWithI18n } from "../../test/i18n";
import { AgentConfigurationPanel } from "./agent-configuration-panel";

vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQuery: () => ({ data: undefined, isSuccess: false }),
}));

vi.mock("@multica/core/config", () => ({
  useConfigStore: (
    selector: (state: { agentConversationStartersSupported: boolean }) => unknown,
  ) => selector({ agentConversationStartersSupported: false }),
}));

vi.mock("../../common/avatar-upload-control", () => ({
  AvatarUploadControl: () => <div data-testid="avatar-upload" />,
}));

vi.mock("../components/runtime-picker", () => ({
  RuntimePicker: () => <div data-testid="runtime-picker" />,
}));

vi.mock("../components/skill-multi-select", () => ({
  SkillMultiSelect: () => <div data-testid="skill-multi-select" />,
}));

vi.mock("../components/model-dropdown", () => ({
  ModelDropdown: () => <div data-testid="model-dropdown" />,
}));

vi.mock("../components/inspector/thinking-prop-row", () => ({
  ThinkingSettingField: () => <div data-testid="thinking-field" />,
}));

vi.mock("../components/inspector/service-tier-setting-field", () => ({
  ServiceTierSettingField: () => <div data-testid="service-tier-field" />,
}));

describe("AgentConfigurationPanel message instructions", () => {
  afterEach(() => {
    cleanup();
  });

  it("places an optional textarea under Description", () => {
    renderWithI18n(
      <AgentConfigurationPanel
        draft={EMPTY_AGENT_DRAFT}
        onChange={vi.fn()}
        runtimes={[]}
        runtimesLoading={false}
        members={[]}
        currentUserId="user-1"
        nameError={null}
        onNameChange={vi.fn()}
      />,
    );

    const description = screen.getByLabelText("Description");
    const messageInstructions = screen.getByLabelText("Message instructions");
    expect(messageInstructions).toBeInTheDocument();
    expect(
      screen.getByText("Added to the start of every message this agent receives."),
    ).toBeInTheDocument();
    expect(
      description.compareDocumentPosition(messageInstructions) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(messageInstructions).not.toBeRequired();
  });
});
