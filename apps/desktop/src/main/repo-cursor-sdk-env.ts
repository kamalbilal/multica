import { existsSync, readFileSync } from "fs";
import { join } from "path";

/** Env vars read from the repo root `.env` for cursor_sdk on local dev hosts. */
export const REPO_CURSOR_SDK_ENV_KEYS = [
  "CURSOR_API_KEY",
  "MULTICA_CURSOR_SDK_EXECUTOR",
] as const;

export type RepoCursorSdkEnvKey = (typeof REPO_CURSOR_SDK_ENV_KEYS)[number];

function detectEnvFilePath(repoRoot: string): string | null {
  const dotEnv = join(repoRoot, ".env");
  if (existsSync(dotEnv)) return dotEnv;
  const worktreeEnv = join(repoRoot, ".env.worktree");
  if (existsSync(worktreeEnv)) return worktreeEnv;
  return null;
}

/**
 * Parse `KEY=value` lines for cursor_sdk daemon vars only. Supports optional
 * single/double quotes; does not evaluate escapes beyond stripping quotes.
 */
export function parseRepoCursorSdkEnvText(text: string): Partial<
  Record<RepoCursorSdkEnvKey, string>
> {
  const allowed = new Set<string>(REPO_CURSOR_SDK_ENV_KEYS);
  const out: Partial<Record<RepoCursorSdkEnvKey, string>> = {};

  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;

    const eq = line.indexOf("=");
    if (eq <= 0) continue;

    const key = line.slice(0, eq).trim();
    if (!allowed.has(key)) continue;

    let value = line.slice(eq + 1).trim();
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1);
    }
    if (value.length === 0) continue;

    out[key as RepoCursorSdkEnvKey] = value;
  }

  return out;
}

export function readRepoCursorSdkEnv(
  repoRoot: string,
): Partial<Record<RepoCursorSdkEnvKey, string>> {
  const envPath = detectEnvFilePath(repoRoot);
  if (!envPath) return {};
  try {
    return parseRepoCursorSdkEnvText(readFileSync(envPath, "utf8"));
  } catch {
    return {};
  }
}

/** Apply repo `.env` cursor_sdk vars when `process.env` does not already set them. */
export function applyRepoCursorSdkEnvToProcess(
  repoRoot: string,
  env: NodeJS.ProcessEnv = process.env,
): void {
  const fromFile = readRepoCursorSdkEnv(repoRoot);
  for (const key of REPO_CURSOR_SDK_ENV_KEYS) {
    const value = fromFile[key];
    if (!value) continue;
    if (env[key]?.trim()) continue;
    env[key] = value;
  }
}

export function repoRootFromDesktopAppPath(appPath: string): string {
  return join(appPath, "..", "..");
}
