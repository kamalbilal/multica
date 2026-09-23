import "./ipc-stdout-guard.js";
import readline from "node:readline";
import type { IpcCommand } from "./protocol.js";
import { writeIpcLine } from "./ipc-stdout-guard.js";
import { dispatchCommand } from "./executor.js";

function emit(event: unknown): void {
  writeIpcLine(`${JSON.stringify(event)}\n`);
}

function logError(message: string): void {
  process.stderr.write(`${message}\n`);
}

async function main(): Promise<void> {
  const rl = readline.createInterface({
    input: process.stdin,
    crlfDelay: Infinity,
  });

  let commandQueue = Promise.resolve();

  for await (const line of rl) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }

    let cmd: IpcCommand;
    try {
      cmd = JSON.parse(trimmed) as IpcCommand;
    } catch (err) {
      logError(err instanceof Error ? err.message : String(err));
      continue;
    }

    const run = commandQueue.then(() => dispatchCommand(cmd, emit));
    commandQueue = run.catch((err) => {
      logError(err instanceof Error ? err.message : String(err));
    });

    if (cmd.cmd === "shutdown") {
      await run;
      process.exit(0);
    }
  }
}

main().catch((err) => {
  logError(err instanceof Error ? err.message : String(err));
  process.exit(1);
});
