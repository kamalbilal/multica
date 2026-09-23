# Cursor SDK Provider (`cursor_sdk`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a local-only `cursor_sdk` Multica agent provider that runs `@cursor/sdk` via a Node executor while preserving all daemon-prepared context (skills, MCP, brief, supplements).

**Architecture:** Go `cursor_sdk` backend spawns `fork/packages/cursor-sdk-executor`, communicates over JSONL stdin/stdout, maps SDK stream events to `agent.Message`/`Result`, and wires `run.steer()` to `Session.Supplement`. Daemon `execenv` treats `cursor_sdk` like `cursor` for filesystem prep.

**Tech Stack:** Go 1.26.6 (Chi daemon), Node 22+, `@cursor/sdk`, TypeScript, Vitest (executor), Go `testing` (backend), PostgreSQL migration 502.

## Global Constraints

- **Local SDK only** — never pass `cloud: { repos }` to `@cursor/sdk`.
- **Coexist with `cursor`** — do not remove or auto-migrate CLI provider.
- **Session IDs** — store SDK `agentId` (`agent-…`), not CLI `session_id`.
- **`cwd` invariant** — SDK `local.cwd` MUST be daemon-prepared workdir from `execenv.Prepare()`.
- **`settingSources`** — default `["project"]` so daemon-written `.cursor/*` and skills load.
- **`CURSOR_API_KEY`** — required on runtime; read from process env (agent `custom_env` or daemon env).
- **Executor path** — `MULTICA_CURSOR_SDK_EXECUTOR` overrides; default `node` + bundled `fork/packages/cursor-sdk-executor/dist/cli.js`.
- **`messages.list` backfill** — only when resume fails or stream ends without terminal result (not every task).
- **No `systemPrompt` override** — Multica brief stays in `AGENTS.md`; ignore daemon `SystemPrompt` for `cursor_sdk`.
- **Fork isolation** — `@cursor/sdk` dependency lives only in `fork/packages/cursor-sdk-executor/package.json`.
- **Migration number** — `502_runtime_profile_add_cursor_sdk` (next after `501`).

## File Map

| File | Responsibility |
|------|----------------|
| `fork/packages/cursor-sdk-executor/package.json` | Pin `@cursor/sdk`, build scripts |
| `fork/packages/cursor-sdk-executor/src/protocol.ts` | Shared command/event types |
| `fork/packages/cursor-sdk-executor/src/stream-mapper.ts` | `SDKMessage` → IPC `message` events |
| `fork/packages/cursor-sdk-executor/src/executor.ts` | Read JSONL commands, run SDK, write events |
| `fork/packages/cursor-sdk-executor/src/cli.ts` | Entry point (stdin/stdout loop) |
| `fork/packages/cursor-sdk-executor/src/stream-mapper.test.ts` | Mapper unit tests |
| `fork/packages/cursor-sdk-executor/src/executor.test.ts` | Executor tests with mocked SDK |
| `server/pkg/agent/cursor_sdk_protocol.go` | Go command/event structs + JSON encode/decode |
| `server/pkg/agent/cursor_sdk_ipc.go` | Spawn executor, read/write JSONL |
| `server/pkg/agent/cursor_sdk.go` | `Backend.Execute`, supplement, cancel |
| `server/pkg/agent/cursor_sdk_test.go` | Fake-executor IPC + mapping tests |
| `server/pkg/agent/cursor_sdk_models.go` | `discoverCursorSdkModels` via executor `list-models` cmd |
| `server/migrations/502_runtime_profile_add_cursor_sdk.up.sql` | `protocol_family` CHECK |
| `server/migrations/502_runtime_profile_add_cursor_sdk.down.sql` | Revert CHECK |
| `fork/patches/cursor-sdk-provider.md` | Upstream touch list |
| `fork/docs/cursor-sdk-selfhost.md` | Operator runbook |

---

### Task 0: Phase 0 spike — resume + IPC proof

**Files:**
- Create: `fork/packages/cursor-sdk-executor/scripts/spike-resume.mjs`
- Create: `fork/docs/cursor-sdk-spike-notes.md`

**Interfaces:**
- Produces: documented answers for workdir resume behavior and minimum SDK version pin.

- [ ] **Step 1: Write spike script**

Create `fork/packages/cursor-sdk-executor/scripts/spike-resume.mjs`:

```javascript
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
```

- [ ] **Step 2: Run spike**

```bash
cd fork/packages/cursor-sdk-executor
npm init -y && npm install @cursor/sdk
export CURSOR_API_KEY="your-key"
node scripts/spike-resume.mjs
```

Expected: `agentId` printed; same-cwd resume `finished`; document behavior for different cwd and post-`rm` cwd in `fork/docs/cursor-sdk-spike-notes.md`.

- [ ] **Step 3: Commit**

```bash
git add fork/packages/cursor-sdk-executor/scripts/spike-resume.mjs fork/docs/cursor-sdk-spike-notes.md
git commit -m "spike: validate Cursor SDK local resume against workdir lifecycle"
```

---

### Task 1: Executor protocol + stream mapper

**Files:**
- Create: `fork/packages/cursor-sdk-executor/package.json`
- Create: `fork/packages/cursor-sdk-executor/tsconfig.json`
- Create: `fork/packages/cursor-sdk-executor/vitest.config.ts`
- Create: `fork/packages/cursor-sdk-executor/src/protocol.ts`
- Create: `fork/packages/cursor-sdk-executor/src/stream-mapper.ts`
- Create: `fork/packages/cursor-sdk-executor/src/stream-mapper.test.ts`

**Interfaces:**
- Produces: `mapSdkMessage(event: SDKMessage): IpcMessageEvent | null`
- Produces: types `ExecuteCommand`, `SteerCommand`, `IpcResultEvent`

- [ ] **Step 1: Scaffold package**

`fork/packages/cursor-sdk-executor/package.json`:

```json
{
  "name": "@multica-fork/cursor-sdk-executor",
  "private": true,
  "type": "module",
  "engines": { "node": ">=22" },
  "dependencies": {
    "@cursor/sdk": "^0.1.0"
  },
  "devDependencies": {
    "typescript": "^5.9.3",
    "vitest": "^4.1.0"
  },
  "scripts": {
    "build": "tsc -p tsconfig.json",
    "test": "vitest run",
    "start": "node dist/cli.js"
  }
}
```

(Pin exact `@cursor/sdk` version from spike Task 0.)

- [ ] **Step 2: Write failing mapper test**

`fork/packages/cursor-sdk-executor/src/stream-mapper.test.ts`:

```typescript
import { describe, expect, it } from "vitest";
import { mapSdkMessage } from "./stream-mapper.js";

describe("mapSdkMessage", () => {
  it("maps assistant text blocks", () => {
    const out = mapSdkMessage({
      type: "assistant",
      agent_id: "agent-1",
      run_id: "run-1",
      message: {
        role: "assistant",
        content: [{ type: "text", text: "hello" }],
      },
    });
    expect(out).toEqual({
      event: "message",
      type: "assistant",
      content: "hello",
    });
  });

  it("maps tool_call completed", () => {
    const out = mapSdkMessage({
      type: "tool_call",
      agent_id: "agent-1",
      run_id: "run-1",
      name: "grep",
      status: "completed",
      call_id: "c1",
      args: { pattern: "foo" },
      result: { matches: 1 },
    });
    expect(out?.type).toBe("tool_result");
    expect(out?.tool).toBe("grep");
    expect(out?.callId).toBe("c1");
  });
});
```

- [ ] **Step 3: Run test — expect FAIL**

```bash
cd fork/packages/cursor-sdk-executor && npm install && npm test
```

Expected: FAIL — `mapSdkMessage` not defined.

- [ ] **Step 4: Implement protocol + mapper**

`fork/packages/cursor-sdk-executor/src/protocol.ts` — export command/event interfaces matching design spec §10.

`fork/packages/cursor-sdk-executor/src/stream-mapper.ts` — implement `mapSdkMessage` for: `assistant`, `thinking`, `tool_call` (started/completed), `status`, `usage` (passthrough for Go token accounting).

- [ ] **Step 5: Run test — expect PASS**

```bash
cd fork/packages/cursor-sdk-executor && npm test
```

- [ ] **Step 6: Commit**

```bash
git add fork/packages/cursor-sdk-executor/
git commit -m "feat(cursor-sdk): add IPC protocol types and SDK stream mapper"
```

---

### Task 2: Node executor CLI

**Files:**
- Create: `fork/packages/cursor-sdk-executor/src/executor.ts`
- Create: `fork/packages/cursor-sdk-executor/src/cli.ts`
- Create: `fork/packages/cursor-sdk-executor/src/executor.test.ts`

**Interfaces:**
- Consumes: `mapSdkMessage`, protocol types from Task 1
- Produces: CLI reading JSONL stdin, writing JSONL stdout; handles `execute`, `steer`, `cancel`, `reload`, `shutdown`, `list-models`

- [ ] **Step 1: Write failing executor test with mocked SDK**

Mock `@cursor/sdk` in `executor.test.ts` — verify `execute` command emits `agent_id`, `message`, `result` sequence.

- [ ] **Step 2: Implement `executor.ts`**

Core logic:

```typescript
// Pseudocode — implement fully in executor.ts
async function handleExecute(cmd: ExecuteCommand, emit: (e: unknown) => void) {
  const apiKey = process.env[cmd.apiKeyEnv ?? "CURSOR_API_KEY"];
  if (!apiKey?.trim()) {
    emit({ event: "error", message: "missing CURSOR_API_KEY", retryable: false });
    return;
  }
  const agent = cmd.agentId
    ? await Agent.resume(cmd.agentId, { apiKey, model: { id: cmd.model }, local: { cwd: cmd.cwd, settingSources: ["project"], sandboxOptions: cmd.sandboxOptions, customTools: cmd.customTools } })
    : await Agent.create({ apiKey, model: { id: cmd.model }, local: { cwd: cmd.cwd, settingSources: ["project"], sandboxOptions: cmd.sandboxOptions, customTools: cmd.customTools }, mcpServers: cmd.mcpConfig ?? undefined });
  emit({ event: "agent_id", agentId: agent.agentId });
  const run = await agent.send(cmd.prompt, { mcpServers: cmd.mcpConfig ?? undefined });
  for await (const sdkEvent of run.stream()) {
    const mapped = mapSdkMessage(sdkEvent);
    if (mapped) emit(mapped);
  }
  const result = await run.wait();
  emit({
    event: "result",
    status: result.status === "finished" ? "completed" : result.status,
    output: result.result ?? "",
    usage: result.usage,
    error: result.error?.message,
  });
  await agent[Symbol.asyncDispose]();
}
```

Also implement:
- `steer` → `run.steer?.(text)` with active run handle stored in module state
- `cancel` → `run.cancel()` + terminal result
- `reload` → `agent.reload()`
- `list-models` → `Cursor.models.list({ apiKey })` → `{ event: "models", items: [...] }`

- [ ] **Step 3: Implement `cli.ts`**

Read stdin line-by-line, `JSON.parse`, dispatch, `JSON.stringify` + `\n` to stdout. Log errors to stderr.

- [ ] **Step 4: Run tests + manual smoke**

```bash
cd fork/packages/cursor-sdk-executor && npm test && npm run build
echo '{"cmd":"shutdown","id":"1"}' | node dist/cli.js
```

- [ ] **Step 5: Commit**

```bash
git add fork/packages/cursor-sdk-executor/src/executor.ts fork/packages/cursor-sdk-executor/src/cli.ts fork/packages/cursor-sdk-executor/src/executor.test.ts
git commit -m "feat(cursor-sdk): add Node executor CLI with execute/steer/cancel/reload"
```

---

### Task 3: Go IPC client + protocol structs

**Files:**
- Create: `server/pkg/agent/cursor_sdk_protocol.go`
- Create: `server/pkg/agent/cursor_sdk_ipc.go`
- Create: `server/pkg/agent/cursor_sdk_ipc_test.go`

**Interfaces:**
- Produces: `type CursorSdkClient struct { ... }`
- Produces: `func NewCursorSdkClient(ctx context.Context, executorPath string, logger *slog.Logger) (*CursorSdkClient, error)`
- Produces: `func (c *CursorSdkClient) Execute(ctx context.Context, req CursorSdkExecuteRequest, onEvent func(CursorSdkEvent)) (CursorSdkResult, error)`
- Produces: `func (c *CursorSdkClient) Steer(ctx context.Context, text string) error`
- Produces: `func (c *CursorSdkClient) Cancel(ctx context.Context) error`
- Produces: `func (c *CursorSdkClient) Reload(ctx context.Context) error`
- Produces: `func (c *CursorSdkClient) Close() error`

- [ ] **Step 1: Write failing IPC test with fake executor script**

`cursor_sdk_ipc_test.go` — create temp shell/node script that echoes fixed JSONL lines for one `execute` command.

- [ ] **Step 2: Implement protocol structs** matching spec §10 field names exactly.

- [ ] **Step 3: Implement IPC client**

Resolve executor:
1. `os.Getenv("MULTICA_CURSOR_SDK_EXECUTOR")` if set
2. Else `node` + path relative to daemon/cwd: `fork/packages/cursor-sdk-executor/dist/cli.js` (document in runbook; use `ExecutablePath` from `Config` when set)

Spawn with `exec.CommandContext`, stdin/stdout pipes, line scanner for events.

- [ ] **Step 4: Run test**

```bash
cd server && go test ./pkg/agent -run TestCursorSdkIPC -v
```

- [ ] **Step 5: Commit**

```bash
git add server/pkg/agent/cursor_sdk_protocol.go server/pkg/agent/cursor_sdk_ipc.go server/pkg/agent/cursor_sdk_ipc_test.go
git commit -m "feat(cursor_sdk): add JSONL IPC client for Node executor"
```

---

### Task 4: Go `cursor_sdk` backend — execute + event mapping

**Files:**
- Create: `server/pkg/agent/cursor_sdk.go`
- Create: `server/pkg/agent/cursor_sdk_test.go`
- Modify: `server/pkg/agent/agent.go` — register `cursor_sdk` in `New()` and `SupportedTypes`
- Modify: `server/pkg/agent/launch.go` — add `cursor_sdk` launch header if needed

**Interfaces:**
- Consumes: `CursorSdkClient` from Task 3
- Produces: `type cursorSdkBackend struct` implementing `Backend`
- Produces: `func (b *cursorSdkBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)`

- [ ] **Step 1: Write failing test `TestCursorSdkExecuteMapsMessages`**

Use fake executor fixture; assert `Message` types and `Result.SessionID` from `agent_id` event.

- [ ] **Step 2: Implement `cursor_sdk.go`**

Map IPC events to existing types (mirror `cursor.go` patterns):

| IPC `message.type` | `agent.MessageType` |
|--------------------|---------------------|
| `assistant` | `MessageAssistant` |
| `thinking` | `MessageThinking` |
| `tool_use` | `MessageToolUse` |
| `tool_result` | `MessageToolResult` |
| `status` | `MessageStatus` |
| `error` | `MessageError` |

Parse `usage` from `result` into `map[string]TokenUsage` like `cursor.go` result handler.

Read sandbox/custom tools from `opts` / env passthrough:
- `CURSOR_SDK_SANDBOX_ENABLED` → `{ enabled: true }`
- `CURSOR_SDK_CUSTOM_TOOLS_JSON` → parse JSON for `customTools`

- [ ] **Step 3: Register provider in `agent.go`**

Add `"cursor_sdk"` to `SupportedTypes` slice and:

```go
case "cursor_sdk":
    return &cursorSdkBackend{cfg: cfg}, nil
```

Add launch header: `"cursor-sdk (local @cursor/sdk)"`.

- [ ] **Step 4: Run tests**

```bash
cd server && go test ./pkg/agent -run CursorSdk -v
```

- [ ] **Step 5: Commit**

```bash
git add server/pkg/agent/cursor_sdk.go server/pkg/agent/cursor_sdk_test.go server/pkg/agent/agent.go server/pkg/agent/launch.go
git commit -m "feat(cursor_sdk): add Go backend with IPC execute and event mapping"
```

---

### Task 5: Database migration + provider registration sweep

**Files:**
- Create: `server/migrations/502_runtime_profile_add_cursor_sdk.up.sql`
- Create: `server/migrations/502_runtime_profile_add_cursor_sdk.down.sql`
- Modify: `server/pkg/agent/agent_supported_types_test.go` (auto via SupportedTypes)
- Modify: `server/internal/metrics/labels.go` — add `cursor_sdk`
- Modify: `server/pkg/agent/launch.go` — `cursorSdkBlockedArgs` if any
- Modify: `server/pkg/agent/thinking_test.go` — add `cursor_sdk` to provider list if applicable

**Interfaces:**
- Produces: DB accepts `protocol_family = 'cursor_sdk'`

- [ ] **Step 1: Write migration** (copy pattern from `441_runtime_profile_add_codearts.up.sql`, add `'cursor_sdk'` to IN list)

- [ ] **Step 2: Add metrics label** in `labels.go`:

```go
"cursor_sdk": "cursor_sdk",
```

- [ ] **Step 3: Run migration + lockstep test**

```bash
make dev  # or run migrations per CONTRIBUTING.md
cd server && go test ./pkg/agent -run TestSupportedTypesLockstepWithNew -v
```

- [ ] **Step 4: Commit**

```bash
git add server/migrations/502_* server/internal/metrics/labels.go
git commit -m "feat(cursor_sdk): add protocol_family migration and metrics label"
```

---

### Task 6: Daemon + execenv wiring

**Files:**
- Modify: `server/internal/daemon/execenv/execenv.go` — `provider == "cursor_sdk"` for MCP prep
- Modify: `server/internal/daemon/execenv/runtime_config.go` — `cursor_sdk` uses `AGENTS.md` path like `cursor`
- Modify: `server/internal/daemon/runtime_mcp.go` — include `cursor_sdk` in cursor branches
- Modify: `server/internal/daemon/daemon.go` — pass `CURSOR_DATA_DIR` for `cursor_sdk`
- Modify: `server/internal/daemon/local_skills.go` — `cursor_sdk` uses `.cursor/skills` discovery roots
- Modify: `server/internal/daemon/agents_probe.go` — probe `cursor_sdk` when executor + Node available
- Modify: `server/internal/daemon/execenv/sidecar_manifest_test.go` — add `cursor_sdk` to `allFileBasedProviders` if listed

**Interfaces:**
- Consumes: `cursor_sdk` provider string from task claim path
- Produces: prepared workdir identical to `cursor` for same task inputs

- [ ] **Step 1: Add helper** in execenv:

```go
func isCursorFamilyProvider(provider string) bool {
    return provider == "cursor" || provider == "cursor_sdk"
}
```

Replace `provider == "cursor"` checks in MCP/runtime paths with helper.

- [ ] **Step 2: Probe logic in `agents_probe.go`**

```go
if nodeOK() && executorOK() {
    agents["cursor_sdk"] = RuntimeAgentEntry{Path: executorPath, ...}
}
```

`nodeOK`: `exec.LookPath("node")` + version ≥ 22.  
`executorOK`: `MULTICA_CURSOR_SDK_EXECUTOR` or default dist path exists.

- [ ] **Step 3: Extend execenv tests**

Copy `cursor_mcp` test case with `provider: "cursor_sdk"`.

- [ ] **Step 4: Run tests**

```bash
cd server && go test ./internal/daemon/execenv/... -run Cursor -v
cd server && go test ./internal/daemon/... -run Probe -v
```

- [ ] **Step 5: Commit**

```bash
git add server/internal/daemon/ server/internal/daemon/execenv/
git commit -m "feat(cursor_sdk): wire daemon execenv and runtime probe"
```

---

### Task 7: Supplements (steer) + cancel + reload

**Files:**
- Modify: `server/pkg/agent/cursor_sdk.go` — `Session.Supplement`, `SupplementReady`
- Modify: `server/pkg/agent/version.go` — `SupportsTaskSupplement` for `cursor_sdk`
- Modify: `server/pkg/agent/cursor_sdk_ipc.go` — steer/cancel/reload commands
- Modify: `fork/packages/cursor-sdk-executor/src/executor.ts` — track active run for steer

**Interfaces:**
- Produces: `Session.Supplement func(context.Context, string) error` calling IPC `steer`
- Produces: `Session.SupplementReady func() bool` true when run status is `running`
- Produces: context cancel → IPC `cancel` before `Close`

- [ ] **Step 1: Write failing test `TestCursorSdkSupplementSteers`**

Pattern from `codex.go` supplement tests.

- [ ] **Step 2: Implement supplement + cancel in Go backend**

On `runCtx.Done()`, call `client.Cancel()` then drain result.

- [ ] **Step 3: Enable `SupportsTaskSupplement`**

```go
case "cursor_sdk":
    return true
```

- [ ] **Step 4: Wire `agent.reload()` from execenv reuse**

In daemon reuse path where cursor MCP refresh runs, if provider is `cursor_sdk`, call backend reload hook (may require storing client ref — use executor `reload` command on same process).

- [ ] **Step 5: Run tests + commit**

```bash
cd server && go test ./pkg/agent -run 'CursorSdk|TaskSupplement' -v
git commit -am "feat(cursor_sdk): wire steer supplements, cancel, and reload"
```

---

### Task 8: Model discovery + messages.list backfill

**Files:**
- Create: `server/pkg/agent/cursor_sdk_models.go`
- Modify: `server/pkg/agent/models.go` — `case "cursor_sdk":`
- Modify: `fork/packages/cursor-sdk-executor/src/executor.ts` — `list-models` + `messages-list` commands

**Interfaces:**
- Produces: `func discoverCursorSdkModels(ctx context.Context, executorPath string) (Catalog, error)`
- Produces: IPC `list-models` → models array
- Produces: IPC `messages-list` → backfill only when resume fails (Go decides when to call)

- [ ] **Step 1: Implement `list-models` in executor** using `Cursor.models.list({ apiKey })`

- [ ] **Step 2: Implement `discoverCursorSdkModels` in Go** — spawn short-lived client, send `list-models`, parse response.

- [ ] **Step 3: Implement `messages-list` in executor** using `Agent.messages.list(agentId, { runtime: "local", cwd })`

- [ ] **Step 4: Go backfill** — in `cursor_sdk.go`, if `opts.ResumeSessionID != ""` and execute returns resume error, call `messages-list` and emit synthetic assistant messages before failing (or after continuity notice path).

- [ ] **Step 5: Tests + commit**

```bash
cd server && go test ./pkg/agent -run 'CursorSdkModels|discoverCursorSdk' -v
git commit -am "feat(cursor_sdk): add SDK model discovery and messages.list backfill"
```

---

### Task 9: Frontend + types

**Files:**
- Modify: `packages/core/types/agent.ts` — add `"cursor_sdk"` to `RUNTIME_PROFILE_PROTOCOL_FAMILIES`
- Modify: `packages/core/agents/mcp-support.ts` — add `"cursor_sdk"` (same as cursor)
- Modify: `packages/views/runtimes/components/provider-logo.tsx` — `case "cursor_sdk": return <CursorLogo />`
- Modify: `packages/views/runtimes/components/runtime-profile-catalog.ts` — `cursor_sdk: "Cursor (SDK)"` in `RUNTIME_TYPE_LABELS`
- Modify: `packages/views/agents/components/agent-overview-pane.test.tsx` — add case
- Modify: agent create UI copy — hint that `CURSOR_API_KEY` is required (find create flow in `packages/views/agents/components/create-agent-dialog.tsx`)

**Interfaces:**
- Produces: `RuntimeProtocolFamily` includes `cursor_sdk`
- Produces: UI label "Cursor (SDK)"

- [ ] **Step 1: Update types array** after `"cursor"` entry:

```typescript
"cursor_sdk",
```

- [ ] **Step 2: Add label and logo cases**

- [ ] **Step 3: Add agent create hint** — when provider is `cursor_sdk`, show: "Requires CURSOR_API_KEY on the runtime and Node 22+."

- [ ] **Step 4: Run frontend tests**

```bash
pnpm test --filter @multica/views -- runtime-profile
pnpm test --filter @multica/core -- runtime-profile-schema
```

- [ ] **Step 5: Commit**

```bash
git add packages/core/types/agent.ts packages/core/agents/mcp-support.ts packages/views/
git commit -m "feat(cursor_sdk): add frontend types, label, and create-agent hints"
```

---

### Task 10: Documentation + fork patch manifest

**Files:**
- Create: `fork/patches/cursor-sdk-provider.md`
- Create: `fork/docs/cursor-sdk-selfhost.md`
- Modify: `FORK.md` — link to self-host doc

**Interfaces:**
- Produces: operator runbook for Node, API key, executor build

- [ ] **Step 1: Write `fork/patches/cursor-sdk-provider.md`** listing every upstream-touched file with one-line rationale.

- [ ] **Step 2: Write `fork/docs/cursor-sdk-selfhost.md`**

Sections: prerequisites, build executor (`cd fork/packages/cursor-sdk-executor && npm ci && npm run build`), set `MULTICA_CURSOR_SDK_EXECUTOR`, set `CURSOR_API_KEY` in agent custom env, create agent with provider Cursor (SDK), verify vs CLI provider.

- [ ] **Step 3: Manual E2E checklist** (in same doc):

1. `make dev`
2. Build executor
3. Create `cursor_sdk` agent with `CURSOR_API_KEY`
4. Assign issue — verify execution log
5. Add supplement mid-run — verify steer
6. Cancel task — verify clean stop
7. Second turn on same issue — verify `agent-` resume

- [ ] **Step 4: Commit**

```bash
git add fork/patches/cursor-sdk-provider.md fork/docs/cursor-sdk-selfhost.md FORK.md
git commit -m "docs: add cursor_sdk self-host runbook and upstream patch manifest"
```

---

## Spec Coverage Checklist

| Spec § | Task |
|--------|------|
| §3 Goals 1–5 | Tasks 4–10 |
| §4 Non-goals | Enforced in Global Constraints |
| §6 Architecture | Tasks 2–4 |
| §7 Session model | Tasks 0, 4 |
| §8 Feature mapping | Tasks 2, 4, 7, 8 |
| §9 Daemon changes | Task 6 |
| §10 IPC protocol | Tasks 1–3 |
| §11 Error handling | Tasks 2, 4, 7 |
| §12 Testing | All tasks |
| §13 Fork layout | Tasks 1–2, 10 |
| §14 Runtime requirements | Task 10 |
| §17 Open items | Resolved: executor=`MULTICA_CURSOR_SDK_EXECUTOR`; backfill=resume failure only; customTools/sandbox=custom_env keys |

## Self-Review (completed)

- No TBD/TODO placeholders in task steps.
- Type names consistent: `CursorSdkClient`, `ExecuteCommand`, `agent_id` event, `cursor_sdk` provider string throughout.
- Each task ends with test command + commit.
- Spec requirements mapped in checklist above.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-23-cursor-sdk.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration.

2. **Inline Execution** — implement tasks in this session with checkpoints after Tasks 0, 4, 6, and 10.

Which approach do you want?
