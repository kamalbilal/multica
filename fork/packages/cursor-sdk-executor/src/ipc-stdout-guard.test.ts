import { describe, expect, it } from "vitest";

describe("ipc stdout guard", () => {
  it("redirects non-IPC stdout writes to stderr", async () => {
    await import("./ipc-stdout-guard.js");

    const stderrChunks: string[] = [];
    const stderrWrite = process.stderr.write.bind(process.stderr);
    process.stderr.write = ((chunk: string | Uint8Array, ...args: unknown[]) => {
      stderrChunks.push(String(chunk));
      return stderrWrite(chunk, ...(args as []));
    }) as typeof process.stderr.write;

    process.stdout.write("19:50:37.978 INFO LocalCursorRulesService load completed\n");

    expect(stderrChunks.join("")).toContain("LocalCursorRulesService load completed");
  });
});
