# Self-hosting Multica with `cursor_sdk`

The `cursor_sdk` provider runs agents through the official `@cursor/sdk` package (local runtime only). It coexists with the existing `cursor` provider (`cursor-agent` CLI).

## Prerequisites

| Requirement | Notes |
| --- | --- |
| Node.js 22+ | `node --version` must report major ≥ 22 |
| `CURSOR_API_KEY` | Set on the runtime host or in the agent's custom env |
| Built executor | `fork/packages/cursor-sdk-executor/dist/cli.js` |
| Multica dev stack | `make dev` (see [CONTRIBUTING.md](../../CONTRIBUTING.md)) |

## Build the executor

From the repository root:

```bash
make cursor-sdk-executor
```

Or manually:

```bash
cd fork/packages/cursor-sdk-executor
npm ci
npm run build
```

Optional override when the default relative path is wrong for your install layout:

```bash
export MULTICA_CURSOR_SDK_EXECUTOR=/absolute/path/to/fork/packages/cursor-sdk-executor/dist/cli.js
```

## Configure the runtime

1. Ensure `node` is on `PATH` where the Multica daemon runs.
2. Set `CURSOR_API_KEY` in the daemon environment **or** add it to the agent's custom env in the UI.
3. Restart the daemon so the probe can discover `cursor_sdk`.

When the key lives only in agent custom env, refresh the model list on the agent
page so discovery forwards that key to the executor (daemon-level keys apply
automatically).

The daemon probe requires Node 22+ and a readable executor script. When both are present, `cursor_sdk` appears alongside other discovered agents.

## Known parity gaps vs `cursor-agent`

- Background shell interrupt (`InterruptBackgroundTools`) is not wired for
  `cursor_sdk` yet. Long-running shell tools without steady heartbeats may hit
  the idle watchdog sooner than on the CLI provider.

## Create a `cursor_sdk` agent

1. Open **Settings → Agents → Create agent**.
2. Choose provider **Cursor (SDK)** (`cursor_sdk`).
3. Add custom env if needed:

   ```
   CURSOR_API_KEY=your-key-here
   ```

4. Optional advanced keys (custom env):

   | Key | Effect |
   | --- | --- |
   | `CURSOR_SDK_SANDBOX_ENABLED=true` | Enables SDK sandbox options |
   | `CURSOR_SDK_CUSTOM_TOOLS_JSON={...}` | JSON map passed as `local.customTools` |

5. Save the agent and assign it to a runtime profile with `protocol_family = cursor_sdk`.

## Verify vs CLI provider

| Check | `cursor` (CLI) | `cursor_sdk` (SDK) |
| --- | --- | --- |
| Discovery | `cursor-agent` on PATH | Node 22+ + executor script |
| Session ID | CLI `session_id` | SDK `agent-…` id |
| Supplements (steer) | No | Yes (`run.steer()`) |
| Cancel | Process kill | `run.cancel()` |
| Model list | `cursor-agent --list-models` | `Cursor.models.list()` via executor |

Use the CLI provider when you only need basic headless runs. Use SDK when you need steer, cancel, reload, inline MCP, or transcript backfill.

## Manual E2E checklist

1. `make dev`
2. Build executor (`npm ci && npm run build` in `fork/packages/cursor-sdk-executor`)
3. Create a `cursor_sdk` agent with `CURSOR_API_KEY` in custom env
4. Assign an issue — confirm execution log shows assistant output
5. Add a supplement mid-run — confirm steer reaches the agent
6. Cancel the task — confirm clean stop (no orphaned executor)
7. Run a second turn on the same issue — confirm resume uses stored `agent-…` session id

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| `cursor_sdk` not in probe list | Node missing, Node < 22, or executor script not built |
| `missing CURSOR_API_KEY` | Key not in daemon env or agent custom env |
| Resume fails with `Agent … not found` | Workdir changed or was deleted; SDK keys agents by `cwd` |
| Empty model dropdown | Executor unreachable; falls back to minimal static list |

See also [cursor-sdk-spike-notes.md](./cursor-sdk-spike-notes.md) for resume/workdir behavior.
