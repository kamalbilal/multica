/**
 * The local @cursor/sdk runtime logs INFO lines to stdout. The executor IPC
 * channel is stdout-only JSONL, so non-IPC writes are redirected to stderr.
 */
let allowIpcStdout = false;

const originalStdoutWrite = process.stdout.write.bind(process.stdout);

process.stdout.write = function writeGuardedStdout(
  chunk: string | Uint8Array,
  encodingOrCallback?: BufferEncoding | ((err?: Error | null) => void),
  callback?: (err?: Error | null) => void,
): boolean {
  if (allowIpcStdout) {
    return originalStdoutWrite(chunk, encodingOrCallback as BufferEncoding, callback);
  }
  return process.stderr.write(chunk, encodingOrCallback as BufferEncoding, callback);
};

export function writeIpcLine(line: string): void {
  allowIpcStdout = true;
  try {
    originalStdoutWrite(line);
  } finally {
    allowIpcStdout = false;
  }
}
