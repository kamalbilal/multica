import readline from "node:readline";
import type { IpcCommand } from "./protocol.js";
import { dispatchCommand } from "./executor.js";

function emit(event: unknown): void {
  process.stdout.write(`${JSON.stringify(event)}\n`);
}

function logError(message: string): void {
  process.stderr.write(`${message}\n`);
}

async function main(): Promise<void> {
  const rl = readline.createInterface({
    input: process.stdin,
    crlfDelay: Infinity,
  });

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

    if (cmd.cmd === "shutdown") {
      await dispatchCommand(cmd, emit);
      process.exit(0);
    }

    void dispatchCommand(cmd, emit).catch((err) => {
      logError(err instanceof Error ? err.message : String(err));
    });
  }
}

main().catch((err) => {
  logError(err instanceof Error ? err.message : String(err));
  process.exit(1);
});
