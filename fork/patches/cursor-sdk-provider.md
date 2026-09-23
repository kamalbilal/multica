# `cursor_sdk` provider — upstream touch list

Fork-only Node executor lives under `fork/packages/cursor-sdk-executor/`. Every other change below is an upstream file patch required to register and wire the provider.

| File | Rationale |
| --- | --- |
| `server/pkg/agent/cursor_sdk_protocol.go` | JSONL command/event structs shared with the Node executor |
| `server/pkg/agent/cursor_sdk_ipc.go` | Spawn executor, read/write JSONL, skip non-JSON stdout lines from SDK logging |
| `server/pkg/agent/cursor_sdk.go` | `cursor_sdk` backend: execute, event mapping, supplements, resume backfill |
| `server/pkg/agent/cursor_sdk_models.go` | Model discovery via `Cursor.models.list()` |
| `server/pkg/agent/cursor_sdk_test.go` | Backend mapping, supplement, reload, cancel tests |
| `server/pkg/agent/cursor_sdk_ipc_test.go` | IPC client tests with fake executor |
| `server/pkg/agent/cursor_sdk_models_test.go` | Model parsing and messages.list backfill tests |
| `server/pkg/agent/agent.go` | Register `cursor_sdk` in `SupportedTypes`, `New()`, launch headers |
| `server/pkg/agent/launch.go` | Launch prefix blocked-args for `cursor_sdk` |
| `server/pkg/agent/models.go` | `case "cursor_sdk":` model discovery |
| `server/pkg/agent/version.go` | `SupportsTaskSupplement("cursor_sdk")` |
| `server/pkg/agent/version_test.go` | Supplement gate coverage |
| `server/pkg/agent/thinking_test.go` | Provider list includes `cursor_sdk` (no thinking control) |
| `server/migrations/502_runtime_profile_add_cursor_sdk.up.sql` | Allow `protocol_family = 'cursor_sdk'` |
| `server/migrations/502_runtime_profile_add_cursor_sdk.down.sql` | Revert protocol_family CHECK |
| `server/internal/metrics/labels.go` | Metrics label for `cursor_sdk` |
| `server/internal/daemon/execenv/cursor_family.go` | `IsCursorFamilyProvider()` helper |
| `server/internal/daemon/execenv/cursor_family_test.go` | Cursor family helper tests |
| `server/internal/daemon/execenv/execenv.go` | MCP prep for `cursor_sdk` |
| `server/internal/daemon/execenv/runtime_config.go` | `AGENTS.md` injection for `cursor_sdk` |
| `server/internal/daemon/execenv/context.go` | `.cursor/skills` discovery roots |
| `server/internal/daemon/execenv/cursor_mcp_test.go` | MCP prep tests for cursor family |
| `server/internal/daemon/execenv/runtime_config_test.go` | Runtime config tests |
| `server/internal/daemon/execenv/sidecar_manifest_test.go` | File-based provider list |
| `server/internal/daemon/runtime_mcp.go` | Cursor MCP branches include `cursor_sdk` |
| `server/internal/daemon/local_skills.go` | Local skills roots for `cursor_sdk` |
| `server/internal/daemon/daemon.go` | MCP auth source, `McpConfigRefreshed` on reuse, `cursor_sdk` launch/probe bypass for `.js` executor |
| `server/internal/daemon/agents_probe.go` | Probe hook for `cursor_sdk` |
| `server/internal/daemon/agents_probe_cursor_sdk.go` | Node 22+ and executor path probe; `verifyCursorSdkAgentEntry` for Windows launch |
| `server/internal/daemon/agents_probe_cursor_sdk_test.go` | Probe tests |
| `server/internal/daemon/agents_probe_cursor_sdk_runtime_test.go` | Windows-style probe/launch path tests |
| `server/internal/daemon/config.go` | Probe error text and Agents comment mention `cursor_sdk` |
| `server/internal/daemon/daemon_test.go` | `cursor_sdk` in resume-rejection undetectable matrix |
| `server/internal/daemon/local_skills_test.go` | `cursor_sdk` local skill bundle parity with `cursor` |
| `server/internal/daemon/execenv/reply_instructions_test.go` | Provider matrix includes `cursor_sdk` |
| `server/internal/daemon/execenv/execenv_test.go` | Provider matrix includes `cursor_sdk` |
| `scripts/ensure-cursor-sdk-executor.sh` | Unix dev bootstrap: build executor, print path |
| `scripts/ensure-cursor-sdk-executor.ps1` | Windows dev bootstrap: build executor, print path |
| `scripts/dev-env.sh` | Wire executor bootstrap (`.sh` or `.ps1` on Windows shells) |
| `scripts/dev.sh` | Wire executor bootstrap on first-time dev setup |
| `fork/packages/cursor-sdk-executor/src/ipc-stdout-guard.ts` | Redirect non-IPC stdout to stderr (SDK logs break JSONL) |
| `fork/packages/cursor-sdk-executor/src/ipc-stdout-guard.test.ts` | Stdout guard tests |
| `fork/packages/cursor-sdk-executor/src/cli.ts` | Import stdout guard before SDK init |
| `server/internal/handler/runtime_models.go` | Optional `discovery_env` on model-list requests (`CURSOR_API_KEY`) |
| `server/internal/handler/runtime_models_redis_store.go` | Persist `discovery_env` in Redis envelope |
| `server/internal/handler/daemon.go` | Forward `discovery_env` to daemon heartbeat |
| `server/pkg/protocol/messages.go` | `DaemonHeartbeatPendingModelList.discovery_env` |
| `server/pkg/agent/launch.go` | `Command.Launcher` + `Command.DiscoveryEnv` for discovery |
| `packages/core/runtimes/models.ts` | `cursorSdkModelDiscoveryEnv` + query key for agent keys |
| `packages/core/api/client.ts` | POST `discovery_env` on `initiateListModels` |
| `packages/views/agents/components/agent-detail-inspector.tsx` | Forward agent `CURSOR_API_KEY` to model discovery |
| `packages/views/runtimes/utils.ts` | `cursor_sdk` pricing alias to `cursor/*` rows |
| `packages/views/runtimes/utils.test.ts` | `cursor_sdk` pricing parity test |
| `packages/views/runtimes/components/runtime-profile-catalog.test.ts` | `runtimeTypeLabel("cursor_sdk")` |
| `CLI_INSTALL.md` | Fork note for `cursor_sdk` detection |
| `SELF_HOSTING.md` | Fork note linking cursor-sdk self-host runbook |
| `apps/docs/content/docs/providers.mdx` | Cursor (SDK) row in supported tools table |
| `packages/core/types/agent.ts` | `RUNTIME_PROFILE_PROTOCOL_FAMILIES` includes `cursor_sdk` |
| `packages/core/agents/mcp-support.ts` | MCP support parity with `cursor` |
| `packages/views/runtimes/components/provider-logo.tsx` | Cursor logo for `cursor_sdk` |
| `packages/views/runtimes/components/runtime-profile-catalog.ts` | Label "Cursor (SDK)" |
| `packages/views/agents/components/create-agent-dialog.tsx` | `CURSOR_API_KEY` / Node 22 hint |
| `packages/views/agents/components/agent-overview-pane.test.tsx` | Overview pane coverage |

## Re-apply after upstream sync

1. Merge or rebase `upstream/main`.
2. Re-apply conflicts using this manifest and `fork/docs/cursor-sdk-selfhost.md`.
3. Rebuild the executor: `cd fork/packages/cursor-sdk-executor && npm ci && npm run build`.
4. Run `cd server && go test ./pkg/agent -run CursorSdk -v` and frontend runtime-profile tests.
5. Optional CI for the executor: `cd fork/packages/cursor-sdk-executor && npm ci && npm test && npm run build`.
