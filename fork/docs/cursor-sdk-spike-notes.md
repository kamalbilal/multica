# Cursor SDK spike notes — local resume + workdir lifecycle

Phase 0 spike for the [cursor-sdk plan](../../docs/superpowers/plans/2026-09-23-cursor-sdk.md). Script: `fork/packages/cursor-sdk-executor/scripts/spike-resume.mjs`.

## Environment

| Item | Value |
| --- | --- |
| `@cursor/sdk` version | `1.0.32` (exact pin) |
| Node | v24.18.0 |
| OS | Windows 10 |
| Spike run | **Ran** with `CURSOR_API_KEY` set (2026-09-23) |

## agentId format

Live output:

```
agentId agent-d263fb41-b5ad-42e8-a936-f714a74893d9 cwd1 C:\Users\kamal\AppData\Local\Temp\multica-spike-W4SxKI
```

- Prefix: `agent-`
- Body: UUID (`xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`)
- Stable across `Agent.create` and `Agent.resume` for the same local agent/workdir pair
- **Store this string as Multica `SessionID`** — not CLI `session_id` (see plan § Session IDs)

## Same-cwd resume

```
same cwd resume finished
```

`Agent.resume(agentId, { local: { cwd: cwd1, ... } })` with the **same** `cwd` used at create time succeeds. Follow-up run status is `finished`. Conversation context is preserved (second prompt answered without re-stating prior context).

Matches SDK docs: resume reattaches to the existing local agent when the workdir path matches.

## Different-cwd resume

```
different cwd resume failed as expected Agent agent-d263fb41-b5ad-42e8-a936-f714a74893d9 not found
```

`Agent.resume` with a **different** `local.cwd` throws (caught in spike). Error message: `Agent <agentId> not found`.

SDK docs ([advanced.md](https://cursor.com/docs/api/sdk/typescript)): for local runtime, the agent is keyed by **path**. A mismatched or missing cwd cannot resolve the agent.

## Post-`rm` cwd (workdir deleted)

Not exercised end-to-end in this run: cleanup (`rm` on `cwd1`) failed with `EBUSY: resource busy or locked` on Windows while SDK agent handles were still open (`await using` disposes at block exit, after `rm`).

**Expected behavior (from SDK docs):** if the workdir path no longer exists, `Agent.resume` cannot find the local agent — same class of failure as different-cwd.

**Implication:** Multica must keep the session workdir alive for the lifetime of a resumable SDK session, or treat resume failure as session loss and surface continuity/backfill paths (plan Task 5).

## Implications for Multica SessionID storage

1. **Persist `agent-…` IDs** in task/run session metadata (`Result.SessionID`), not CLI session identifiers.
2. **Bind SessionID to workdir path** — resume requires the same `local.cwd` (or a path that resolves to the same agent). Store cwd alongside SessionID in executor state / Go `cursor_sdk` options.
3. **Do not delete workdir** while a session may be resumed; align workdir lifecycle with agent disposal or explicit session end.
4. **Resume errors are recoverable signals** — `Agent … not found` means wrong/missing cwd or expired local agent; plan backfill via `Agent.messages.list` when resume fails but history is still needed.
5. **Cross-process resume is viable** — `Agent.resume` works from a fresh Node process as long as apiKey, agentId, and cwd match.

## How to re-run

```bash
cd fork/packages/cursor-sdk-executor
npm install
export CURSOR_API_KEY="..."
node scripts/spike-resume.mjs
```
