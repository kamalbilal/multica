# Cursor SDK Provider for Multica (Local) — Design Spec

**Date:** 2026-09-23  
**Status:** Approved (brainstorming)  
**Fork:** [kamalbilal/multica](https://github.com/kamalbilal/multica)  
**Author:** Design session with project owner

---

## 1. Summary

Add a new Multica agent provider **`cursor_sdk`** that runs agents via the official **`@cursor/sdk`** TypeScript package (local runtime only). It coexists with the existing **`cursor`** provider (`cursor-agent` CLI). The daemon continues to own all Multica context preparation (skills, MCP files, runtime brief, project resources, prompts). A new Go backend spawns a Node executor that wraps the SDK and speaks a JSONL IPC protocol.

**Deployment target:** self-hosted Multica only. Operator controls runtime machines (Node 22+, `CURSOR_API_KEY`).

---

## 2. Problem statement

Multica already integrates Cursor through the `cursor-agent` CLI (`server/pkg/agent/cursor.go`). That path cannot expose the full local Cursor SDK surface:

| Capability | `cursor` (CLI) | `@cursor/sdk` (local) |
|------------|----------------|------------------------|
| Mid-run steer (supplements) | No | `run.steer()` |
| Clean cancellation | Process kill | `run.cancel()` |
| Config hot-reload | No | `agent.reload()` |
| Inline MCP per run | Files only | `mcpServers` on send |
| Transcript API | Parse stream only | `Agent.messages.list`, `run.conversation()` |
| Custom tools / sandbox | `--yolo` only | `local.customTools`, `local.sandboxOptions` |
| Model catalog | CLI `--list-models` | `Cursor.models.list()` |

Supplements today are limited to codex/claude (`SupportsTaskSupplement` in `server/pkg/agent/version.go`). A SDK-backed Cursor provider can wire `run.steer()` to Multica's supplement loop (same pattern as codex `turn/steer`).

---

## 3. Goals

1. Add **`cursor_sdk`** as a first-class provider alongside **`cursor`**.
2. **Local SDK runtime only** — `local: { cwd, settingSources, sandboxOptions, customTools, store }`.
3. **Preserve all Multica daemon context** — no bypass of `execenv.Prepare()`.
4. **Full local SDK surface in v1:**
   - `Agent.create` / `Agent.resume` / internal one-shot via `send`+`wait`
   - `send` + `stream` + `wait`
   - `run.steer()` → `Session.Supplement`
   - `run.cancel()` → task cancellation
   - `agent.reload()` after daemon MCP refresh
   - Inline `mcpServers` merged with file-based MCP
   - `Agent.messages.list` for transcript recovery on resume gaps
   - `local.customTools` (via agent extended config)
   - `local.sandboxOptions` (via agent config / custom env)
   - `Cursor.models.list()` for model discovery
5. Self-hosted deployments where the operator installs Node and sets `CURSOR_API_KEY` on runtimes.

---

## 4. Non-goals (v1)

- Cloud agents (`bc-*`), PR creation, artifacts
- Replacing or auto-migrating existing `cursor` CLI agents
- Official multica.ai SaaS runtime packaging
- Upstream PR to multica-ai/multica (fork first; contribute later)
- `run.conversation()` in execution log UI (defer to v1.1 if scope-heavy)

---

## 5. Decisions (locked)

| Question | Decision |
|----------|----------|
| Runtime scope | Local SDK only |
| Provider relationship | New `cursor_sdk` coexists with `cursor` |
| MVP depth | Full local SDK surface (see §3) |
| Deployment | Self-host only |
| Architecture | **Approach A:** Go `cursor_sdk` backend + Node executor (JSONL IPC) |
| Session identity | Store SDK `agentId` (`agent-…`), not CLI `session_id` |
| Multica brief delivery | Files (`AGENTS.md`, `.cursor/skills/`) — same as `cursor` provider |

**Rejected approach:** Node CLI shim mimicking `cursor-agent` stream-json — insufficient for steer/cancel/reload/messages.

**Deferred approach:** Long-lived Node executor pool on daemon host — revisit if per-task startup latency is problematic.

---

## 6. Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Multica Server + Daemon                                      │
│  claim task → execenv.Prepare(provider=cursor_sdk)           │
│  → same cursor MCP / skill / brief paths as cursor provider  │
└───────────────────────────┬─────────────────────────────────┘
                            │ agent.Backend.Execute()
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ server/pkg/agent/cursor_sdk.go                               │
│  • spawn fork/packages/cursor-sdk-executor                   │
│  • IPC: execute | steer | cancel | reload | shutdown         │
│  • map SDK stream → agent.Message / Result                   │
│  • Session.Supplement → steer RPC                            │
│  • SessionID = SDK agentId                                   │
└───────────────────────────┬─────────────────────────────────┘
                            │ JSONL stdin/stdout
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ fork/packages/cursor-sdk-executor (Node + @cursor/sdk)       │
│  • Agent.create/resume({ local, mcpServers, ... })         │
│  • agent.send(prompt) → run.stream() → run.wait()            │
└─────────────────────────────────────────────────────────────┘
```

**Invariant:** `local.cwd` passed to the SDK MUST be the daemon-prepared workdir returned by `execenv.Prepare()`, not an arbitrary user path (unless Prepare explicitly used `local_directory` mode).

---

## 7. Session and identity model

| Field | `cursor` (CLI) | `cursor_sdk` (new) |
|-------|----------------|---------------------|
| Stored key | `session_id` from stream-json | `agent.agentId` (`agent-…`) |
| Resume mechanism | `--resume <session_id>` | `Agent.resume(agentId, { local: { cwd } })` |
| DB / task column | `SessionID` | Same column, different format |
| Agent records | `protocol_family: cursor` | `protocol_family: cursor_sdk` |

**Rules:**

- Never mix ID formats on one agent record.
- Existing Cursor CLI agents are not migrated automatically.
- New agents explicitly choose SDK in the provider picker.

**Risk:** If Multica recreates the task workdir between turns, local SDK resume may fail if the agent store is tied to a destroyed path. Mitigation: document behavior; use continuity notice blocks (existing `prompt.go` / `execenv` patterns); spike in Phase 0.

---

## 8. Feature mapping (SDK → Multica)

| SDK capability | Multica integration |
|----------------|---------------------|
| `local.cwd` | `opts.Cwd` from daemon after Prepare |
| `local.settingSources` | Default `["project"]` — loads daemon-written `.cursor/*`, rules, skills |
| `CURSOR_DATA_DIR` | Pass through from `execenv.Environment` (same as cursor) |
| `mcpServers` inline | Parse `opts.McpConfig`; merge at `send()` with file MCP |
| `systemPrompt` | Do not replace `AGENTS.md` runtime brief; ignore unless future explicit opt-in |
| `send` / `stream` | Map to `Message` channel types: assistant, thinking, tool_use, tool_result, status, error |
| `wait` | `Result` channel: status, output, usage, error |
| `run.steer()` | `Session.Supplement` + `SupplementReady`; enable `SupportsTaskSupplement("cursor_sdk", …)` |
| `run.cancel()` | On agent context cancellation / daemon stop-task |
| `agent.reload()` | Invoke when `execenv` reuse path refreshes MCP config |
| `Agent.messages.list` | Backfill transcript when resume fails or stream is incomplete |
| `local.customTools` | Agent extended config JSON (schema defined in implementation plan) |
| `local.sandboxOptions` | Agent config / `custom_env` keys (e.g. `CURSOR_SDK_SANDBOX_ENABLED`) |
| `Cursor.models.list` | `discoverCursorSdkModels()` in `models.go` for `cursor_sdk` provider |
| Token usage | Map SDK `usage` / `result.usage` to existing `TokenUsage` maps |

---

## 9. Daemon and execenv changes

Treat `cursor_sdk` like `cursor` for filesystem and env preparation.

| File | Change |
|------|--------|
| `server/internal/daemon/execenv/execenv.go` | MCP prep when `provider == "cursor_sdk"` |
| `server/internal/daemon/execenv/runtime_config.go` | `AGENTS.md` injection for `cursor_sdk` |
| `server/internal/daemon/execenv/cursor_mcp.go` | No change to logic; called via shared provider branch |
| `server/internal/daemon/runtime_mcp.go` | Include `cursor_sdk` in cursor MCP path branches |
| `server/internal/daemon/daemon.go` | Export `CURSOR_DATA_DIR` for `cursor_sdk` |
| `server/pkg/agent/models.go` | `case "cursor_sdk":` model discovery via SDK list |
| `server/pkg/agent/agent.go` | Register `cursor_sdk` in `New()` + `SupportedTypes` |
| `server/pkg/agent/version.go` | `SupportsTaskSupplement` for `cursor_sdk` |
| `server/migrations/NNN_runtime_profile_add_cursor_sdk.up.sql` | Extend `protocol_family` CHECK |
| Frontend agent provider UI | Label "Cursor (SDK)", require `CURSOR_API_KEY` hint |

---

## 10. IPC protocol (Go ↔ Node)

**Transport:** newline-delimited JSON on child stdin/stdout. stderr = structured logs only.

### Go → Node (commands)

```json
{"cmd":"execute","id":"req-1","prompt":"…","cwd":"/abs/workdir","agentId":"agent-…|null","model":"composer-2.5","mcpConfig":{},"customTools":{},"sandboxOptions":{},"apiKeyEnv":"CURSOR_API_KEY"}
{"cmd":"steer","id":"req-2","text":"…"}
{"cmd":"cancel","id":"req-3"}
{"cmd":"reload","id":"req-4"}
{"cmd":"shutdown","id":"req-5"}
```

### Node → Go (events)

```json
{"event":"agent_id","agentId":"agent-…"}
{"event":"message","type":"assistant","content":"…"}
{"event":"message","type":"thinking","content":"…"}
{"event":"message","type":"tool_use","tool":"…","callId":"…","input":{}}
{"event":"message","type":"tool_result","callId":"…","output":"…"}
{"event":"message","type":"status","status":"running"}
{"event":"result","status":"completed|failed|cancelled","output":"…","usage":{},"error":"…"}
{"event":"error","message":"…","retryable":false}
```

### Process lifecycle

1. Go spawns executor at `Execute()` start (resolve path from config or bundled binary).
2. Go sends `execute`; Node creates or resumes agent, streams events until `result`.
3. On supplement: Go sends `steer` when `SupplementReady()`.
4. On context cancel: Go sends `cancel`, then waits for terminal `result`.
5. On cleanup: Go sends `shutdown`; Node calls `agent[Symbol.asyncDispose]()`.

---

## 11. Error handling

| Failure | Multica behavior |
|---------|------------------|
| Missing `CURSOR_API_KEY` / 401 at startup | `CursorAgentError` → classify as missing config |
| `result.status === "error"` | Task failed; keep partial Messages |
| Executor crash | Same as CLI crash — stderr tail, watchdog |
| Steer when not ready | `SupplementReady() == false`; daemon polls (codex pattern) |
| Resume with unknown `agentId` | Fresh agent + continuity notice in prompt |
| Node binary missing | Runtime probe failure with install instructions |
| IPC protocol error | Fail task with diagnostic; log `requestId` |

Distinguish startup failures (`CursorAgentError`) from run failures (`result.status === "error"`) in executor and map to existing `taskfailure` reasons where possible.

---

## 12. Testing strategy

| Layer | Coverage |
|-------|----------|
| `stream-mapper.ts` | Unit tests with SDK message fixtures |
| `cursor-sdk-executor` | Integration tests with mocked `@cursor/sdk` |
| `cursor_sdk.go` | IPC contract tests with fake executor script |
| `cursor_sdk.go` | Golden event mapping (parallel to `cursor-agent-*.jsonl` fixtures) |
| `execenv` | Extend cursor MCP tests for `cursor_sdk` provider string |
| `version.go` | Supplement gating tests |
| E2E (manual) | Self-host: create SDK agent → assign issue → supplement mid-run → second turn resume |

---

## 13. Fork layout and upstream sync

```
fork/
├── packages/cursor-sdk-executor/
│   ├── package.json              # pins @cursor/sdk
│   ├── src/protocol.ts
│   ├── src/executor.ts
│   ├── src/stream-mapper.ts
│   └── bin/multica-cursor-sdk-executor
├── patches/cursor-sdk-provider.md   # every upstream file touched
server/pkg/agent/cursor_sdk.go       # tracked diff from upstream
server/pkg/agent/cursor_sdk_test.go
server/migrations/NNN_*.sql
```

**Sync:** weekly `scripts/sync-upstream.ps1`. High-conflict files: `agent.go`, `models.go`, `execenv/`.

---

## 14. Runtime requirements (self-host)

| Requirement | Notes |
|-------------|-------|
| Node.js ≥ 22 | On every machine that runs `cursor_sdk` agents |
| `CURSOR_API_KEY` | Runtime `custom_env` or daemon environment |
| Executor binary | Built from `fork/packages/cursor-sdk-executor`; path via env or bundled release |
| `cursor-agent` CLI | **Not** required for `cursor_sdk` agents |

---

## 15. Implementation phases (outline)

Implementation detail lives in the separate implementation plan (`docs/superpowers/plans/2026-09-23-cursor-sdk.md`).

| Phase | Deliverable |
|-------|-------------|
| 0 — Spike | Resume ID + workdir lifecycle proof; IPC hello-world |
| 1 — Executor | Node package + stream mapper + protocol tests |
| 2 — Go backend | `cursor_sdk.go`, IPC integration, event mapping |
| 3 — Daemon wiring | execenv branches, MCP, `CURSOR_DATA_DIR`, migration |
| 4 — Supplements | steer + `SupportsTaskSupplement` |
| 5 — Full surface | cancel, reload, customTools, sandbox, models.list, messages.list |
| 6 — UI + docs | Provider picker, self-host runbook, `fork/patches/` |

---

## 16. Risks and mitigations

| Risk | Mitigation |
|------|------------|
| SDK API churn | Pin `@cursor/sdk` version; isolate in `fork/packages/` |
| Session resume across workdir GC | Phase 0 spike; document limits; continuity notices |
| Node on runtime machines | Clear probe error; document in self-host guide |
| Upstream merge conflicts | Keep Go changes minimal; document patches |
| Two Cursor providers confuse users | UI labels + docs: CLI vs SDK requirements |

---

## 17. Open items (implementation plan)

1. Exact schema for agent extended config (`customTools`, `sandboxOptions`).
2. Executor binary discovery path (`MULTICA_CURSOR_SDK_EXECUTOR` vs relative to daemon).
3. Whether `messages.list` backfill runs on every task or only on resume failure.
4. Runtime probe: version check for Node and SDK executor health.

---

## Appendix A: Reference code locations

| Concern | Path |
|---------|------|
| Existing Cursor CLI backend | `server/pkg/agent/cursor.go` |
| Codex supplement pattern | `server/pkg/agent/codex.go` (`turn/steer`) |
| Cursor MCP prep | `server/internal/daemon/execenv/cursor_mcp.go` |
| Provider whitelist | `server/pkg/agent/agent.go` (`SupportedTypes`) |
| Supplement gating | `server/pkg/agent/version.go` |
| Plugin capabilities (analogy) | `server/pkg/plugincontract/capabilities.go` |
| Example plugins | `examples/plugins/` |
