import { Agent } from "@cursor/sdk";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const apiKey = process.env.CURSOR_API_KEY;
if (!apiKey) {
  console.error("Set CURSOR_API_KEY");
  process.exit(1);
}

const cwd1 = await mkdtemp(join(tmpdir(), "multica-spike-"));
const cwd2 = await mkdtemp(join(tmpdir(), "multica-spike-"));

await using agent = await Agent.create({
  apiKey,
  model: { id: "composer-2" },
  local: { cwd: cwd1, settingSources: ["project"] },
});

const run1 = await agent.send("Reply with exactly: spike-ok");
await run1.wait();
const agentId = agent.agentId;
console.log("agentId", agentId, "cwd1", cwd1);

await using resumed = await Agent.resume(agentId, {
  apiKey,
  model: { id: "composer-2" },
  local: { cwd: cwd1, settingSources: ["project"] },
});
const run2 = await resumed.send("Reply with exactly: spike-resume-ok");
console.log("same cwd resume", (await run2.wait()).status);

try {
  await using wrongCwd = await Agent.resume(agentId, {
    apiKey,
    model: { id: "composer-2" },
    local: { cwd: cwd2, settingSources: ["project"] },
  });
  const run3 = await wrongCwd.send("test");
  console.log("different cwd resume", (await run3.wait()).status);
} catch (err) {
  console.log("different cwd resume failed as expected", err.message);
}

await rm(cwd1, { recursive: true, force: true });
await rm(cwd2, { recursive: true, force: true });
