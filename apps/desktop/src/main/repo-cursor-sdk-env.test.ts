// @vitest-environment node

import { describe, expect, it } from "vitest";
import { parseRepoCursorSdkEnvText } from "./repo-cursor-sdk-env";

describe("parseRepoCursorSdkEnvText", () => {
  it("reads cursor_sdk keys and ignores unrelated lines", () => {
    expect(
      parseRepoCursorSdkEnvText(`
# comment
OTHER=ignored
CURSOR_API_KEY=agent-key
MULTICA_CURSOR_SDK_EXECUTOR=/path/to/cli.js
`),
    ).toEqual({
      CURSOR_API_KEY: "agent-key",
      MULTICA_CURSOR_SDK_EXECUTOR: "/path/to/cli.js",
    });
  });

  it("strips optional quotes", () => {
    expect(
      parseRepoCursorSdkEnvText('CURSOR_API_KEY="quoted-key"\n'),
    ).toEqual({ CURSOR_API_KEY: "quoted-key" });
  });
});
