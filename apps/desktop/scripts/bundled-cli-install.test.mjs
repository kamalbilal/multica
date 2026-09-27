import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";

import {
  clearBundledCliDestination,
  installBundledBinary,
} from "./bundled-cli-install.mjs";

const cleanups = [];
afterEach(() => {
  while (cleanups.length) cleanups.pop()();
});

function tempDir() {
  const root = mkdtempSync(join(tmpdir(), "bundle-cli-"));
  cleanups.push(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

describe("installBundledBinary", () => {
  it("copies when the destination does not exist yet", async () => {
    const root = tempDir();
    const src = join(root, "src.bin");
    const dest = join(root, "bin", "multica.exe");
    writeFileSync(src, "v1");

    await installBundledBinary(src, dest);

    expect(readFileSync(dest, "utf8")).toBe("v1");
  });

  it("replaces an existing destination without removing the parent directory", async () => {
    const root = tempDir();
    const src = join(root, "src.bin");
    const dest = join(root, "bin", "multica.exe");
    writeFileSync(src, "v2");
    mkdirSync(dirname(dest), { recursive: true });
    writeFileSync(dest, "v1");

    await installBundledBinary(src, dest);

    expect(readFileSync(dest, "utf8")).toBe("v2");
  });

  it("clearBundledCliDestination removes swap artifacts", async () => {
    const root = tempDir();
    const dest = join(root, "multica.exe");
    writeFileSync(dest, "x");
    writeFileSync(`${dest}.old`, "y");

    await clearBundledCliDestination(dest);

    expect(() => readFileSync(dest)).toThrow();
  });
});
