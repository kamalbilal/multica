#!/usr/bin/env node
// Fast local desktop: production Electron bundles + preview (no Vite dev server).
// Uses apps/desktop/.env.development.local for fork API URLs (same as dev:desktop).
// Rebuild when UI changes: this script always runs `electron-vite build` first.

import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { envWithLocalBins } from "./package.mjs";
import {
  applyWorktreeDevEnv,
  loadDesktopViteEnv,
  repoRootFromScriptDir,
} from "./worktree-dev-env.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const appDir = join(here, "..");

applyWorktreeDevEnv(process.env, {
  root: repoRootFromScriptDir(here),
  log: true,
});
loadDesktopViteEnv(appDir, process.env);
if (process.env.VITE_API_URL) {
  console.log(
    `[preview:local] API ${process.env.VITE_API_URL} (from .env.development.local)`,
  );
} else {
  console.warn(
    "[preview:local] VITE_API_URL missing — run scripts/sync-desktop-env.bat or just up",
  );
}

function run(command, args, { shell = false, env = process.env } = {}) {
  const result = spawnSync(command, args, {
    stdio: "inherit",
    env,
    shell,
  });
  if (result.error) {
    console.error(
      `[preview:local] failed to run ${command}: ${result.error.message}`,
    );
    process.exit(1);
  }
  if (result.status !== 0) process.exit(result.status ?? 1);
}

const node = process.execPath;
const env = envWithLocalBins(process.env);
const isWin = process.platform === "win32";

console.log("[preview:local] bundling multica CLI…");
run(node, [join(here, "bundle-cli.mjs")], { env });

console.log("[preview:local] building desktop (optimized bundles)…");
run("electron-vite", ["build", "--mode", "development"], { shell: isWin, env });

console.log("[preview:local] starting Electron preview…");
run("electron-vite", ["preview"], { shell: isWin, env });
