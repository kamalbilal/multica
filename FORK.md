# Multica fork maintenance

This repository is a fork of [multica-ai/multica](https://github.com/multica-ai/multica).

| Remote     | URL                                              | Purpose                          |
| ---------- | ------------------------------------------------ | -------------------------------- |
| `origin`   | `https://github.com/kamalbilal/multica.git`      | Your fork (push custom work)     |
| `upstream` | `https://github.com/multica-ai/multica.git`      | Official Multica (pull updates)  |

## Golden rules

1. **Keep custom code in `fork/`** — plugins, packages, docs, and scripts that are unique to this fork live under `fork/`. Upstream never touches that directory, so merges stay clean.
2. **Avoid editing upstream files** when a plugin, env var, or config can do the job instead. Multica ships a plugin system — see `examples/plugins/` and put your plugins in `fork/plugins/`.
3. **When you must patch upstream**, record the change in `fork/patches/` (unified diff or short note) so you can re-apply after each sync.
4. **Sync often** — upstream releases most weekdays; pulling weekly is easier than merging months of drift.

## Branch strategy

| Branch        | Use |
| ------------- | --- |
| `main`        | Tracks upstream `main` plus fork-only commits (setup, `fork/` additions). |
| `feature/*`   | Short-lived branches for larger custom work; merge back to `main`. |

Do **not** force-push `main` unless you know exactly why. Prefer `git merge upstream/main` over rebase if others collaborate on this fork.

## Sync from upstream

### Windows (PowerShell)

```powershell
.\scripts\sync-upstream.ps1
```

### macOS / Linux

```bash
./scripts/sync-upstream.sh
```

### Manual steps

```bash
git fetch upstream
git checkout main
git merge upstream/main
# resolve conflicts if any, then:
git push origin main
```

If you only want to preview what changed:

```bash
git fetch upstream
git log --oneline main..upstream/main
```

## Where to add custom features

| Goal                         | Location / approach |
| ---------------------------- | ------------------- |
| UI panels, issue workflows   | `fork/plugins/<name>/` (zip and upload in Settings → Plugins) |
| Local plugin dev             | Set `MULTICA_PLUGIN_DIR` to your plugin folder |
| New shared TS packages       | `fork/packages/<name>/` and add to `pnpm-workspace.yaml` (document in `fork/patches/`) |
| Backend / Go changes         | Prefer upstream PR; otherwise patch `server/` and save diff under `fork/patches/` |
| Self-host config             | `.env` (never commit secrets); see `SELF_HOSTING.md` |
| `cursor_sdk` provider        | `make cursor-sdk-executor`; see [fork/docs/cursor-sdk-selfhost.md](fork/docs/cursor-sdk-selfhost.md) |
| Cursor SDK provider (`cursor_sdk`) | [fork/docs/cursor-sdk-selfhost.md](fork/docs/cursor-sdk-selfhost.md); upstream touch list in [fork/patches/cursor-sdk-provider.md](fork/patches/cursor-sdk-provider.md) |

## First-time development setup

Prerequisites: Node.js 22, pnpm 10.28.2, Go 1.26.6, Docker, [just](https://github.com/casey/just) (optional).

```bash
make dev
```

### Production self-host (fork)

With [just](https://github.com/casey/just) installed (`winget install Casey.Just`) and Git Bash on `PATH`:

```bash
just build            # compile Go server/CLI + Next.js web (+ cursor_sdk executor)
just up               # start production api, web, and daemon (not dev servers)
just restart          # stop + start again (does not rebuild)
just refresh          # alias for restart
just status           # show ports and running components
just rebuild-restart  # build, then restart
```

Hot reload while developing: `make up` or `make dev`.

See [CONTRIBUTING.md](CONTRIBUTING.md) for worktrees, testing, and troubleshooting.

## Contributing back to upstream

Changes that benefit everyone should go to [multica-ai/multica](https://github.com/multica-ai/multica) as a pull request. Keep fork-only branding, credentials, and private integrations in `fork/`.
