# Upstream patches

When you cannot avoid editing a file outside `fork/`, document it here.

## Format

Create a file named after the upstream path, e.g. `pnpm-workspace.md`:

```markdown
# pnpm-workspace.yaml

**Why:** include fork/packages in the monorepo.

**Change:** add `- fork/packages/*` under `packages:`.

**Last verified with upstream:** main @ <short-sha>
```

For mechanical re-application, store a `.patch` file:

```bash
git diff upstream/main -- path/to/file > fork/patches/path-to-file.patch
git apply fork/patches/path-to-file.patch   # after sync
```
