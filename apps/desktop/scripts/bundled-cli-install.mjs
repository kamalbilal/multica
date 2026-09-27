import { constants, existsSync, readdirSync } from "node:fs";
import { copyFile, mkdir, rename, rm, access } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

const PLATFORM_TO_GOOS = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

export function serverBuiltCliPath(repoRoot) {
  const goos = PLATFORM_TO_GOOS[process.platform];
  if (!goos) {
    throw new Error(`unsupported platform for bundled CLI: ${process.platform}`);
  }
  const goarch = process.arch === "x64" ? "amd64" : process.arch;
  const binName = process.platform === "win32" ? "multica.exe" : "multica";
  return join(repoRoot, "server", "bin", `${goos}-${goarch}`, binName);
}

export function desktopBundledCliPath(repoRoot) {
  const binName = process.platform === "win32" ? "multica.exe" : "multica";
  return join(repoRoot, "apps", "desktop", "resources", "bin", binName);
}

async function pathExists(p) {
  try {
    await access(p, constants.F_OK);
    return true;
  } catch {
    return false;
  }
}

/**
 * Stops Desktop-owned CLI daemons (`desktop-*` profiles) so a rebundle can
 * replace `resources/bin/multica.exe` on Windows without EPERM on unlink.
 */
export function stopDesktopProfileDaemons(cliBinary) {
  if (!existsSync(cliBinary)) return;
  const profilesRoot = join(homedir(), ".multica", "profiles");
  let names = [];
  try {
    names = readdirSync(profilesRoot, { withFileTypes: false });
  } catch {
    return;
  }
  for (const name of names) {
    if (!name.startsWith("desktop-")) continue;
    try {
      execFileSync(cliBinary, ["daemon", "stop", "--profile", name], {
        stdio: "pipe",
        timeout: 20_000,
      });
    } catch {
      // Profile may already be stopped or the binary may be mid-replace.
    }
  }
}

/**
 * On Windows, executables that are still running can be renamed away, then
 * replaced — unlinking the directory (old bundle-cli behavior) fails with
 * EPERM while any child daemon holds the file open.
 */
export async function installBundledBinary(src, dest) {
  await mkdir(dirname(dest), { recursive: true });
  const tmp = `${dest}.new`;
  const stale = `${dest}.old`;

  await copyFile(src, tmp);

  const commit = async () => {
    await rm(stale, { force: true });
    if (await pathExists(dest)) {
      await rename(dest, stale);
    }
    await rename(tmp, dest);
    await rm(stale, { force: true });
  };

  try {
    await commit();
  } catch (err) {
    const retriable =
      err && typeof err === "object" && (err.code === "EPERM" || err.code === "EBUSY");
    if (!retriable) {
      await rm(tmp, { force: true });
      throw err;
    }
    stopDesktopProfileDaemons(src);
    try {
      await commit();
    } catch (retryErr) {
      await rm(tmp, { force: true });
      throw retryErr;
    }
  }
}

export async function clearBundledCliDestination(dest) {
  await rm(dest, { force: true });
  await rm(`${dest}.new`, { force: true });
  await rm(`${dest}.old`, { force: true });
}
