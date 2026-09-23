# Fork customizations

Everything in this directory is **ours** — upstream Multica does not ship these files, so syncing from `multica-ai/multica` will not overwrite them.

## Layout

```
fork/
├── plugins/     # Custom Multica plugins (see examples/plugins/ for manifests)
├── packages/    # Optional pnpm packages (requires pnpm-workspace.yaml entry)
├── patches/     # Diffs or notes for unavoidable edits to upstream files
└── config/      # Fork-specific config templates (no secrets)
```

## Plugins

Each plugin is a folder with a `multica.plugin.json` manifest. Copy `examples/plugins/hello-panel/` as a starting point.

During development:

```bash
export MULTICA_PLUGIN_DIR=/path/to/fork/plugins/my-plugin
```

Zip the folder to install via **Settings → Plugins** in production.

## Packages

If you add TypeScript packages under `fork/packages/`, append to the root `pnpm-workspace.yaml`:

```yaml
packages:
  - apps/*
  - packages/*
  - fork/packages/*
```

Record that one-line workspace change in `fork/patches/pnpm-workspace.md` so you remember to re-apply it after a messy merge.
