---
title: Drift Handling
sidebar:
  order: 3
---

## What is Drift?

Drift occurs when someone manually edits a file that gh-infra manages. For example, a developer pushes a change to `.github/CODEOWNERS` directly, making it different from what your manifest declares.

On the next `plan`, gh-infra detects this difference. The `on_drift` field controls what happens next.

## Options

`on_drift` can be set at the **spec level** (default for all files) or at the **file level** (override for a specific file).

```yaml
spec:
  on_drift: warn    # spec-level default: warn | overwrite | skip

  files:
    - path: .gitignore
      on_drift: overwrite   # file-level override
      content: ...

    - path: LICENSE
      source: ./templates/LICENSE
      # inherits spec-level on_drift: warn
```

### Resolution order

```
mirror (always overwrite) > file-level on_drift > spec-level on_drift > default "warn"
```

### `warn` (default)

Shows a warning during `plan` and **skips the file** during `apply`. This is the safest option — it tells you drift exists without overwriting someone's intentional changes.

Use this when you're not sure whether the manual edit was intentional.

### `overwrite`

Shows the diff during `plan` and **overwrites the file** during `apply`, restoring it to the declared state.

Use this when you want strict enforcement — the YAML is the source of truth, and manual edits should be reverted.

### `skip`

Ignores the file entirely — no warning, no action. gh-infra pretends the file doesn't exist.

Use this when you've intentionally allowed a repo to diverge and don't want noise in the plan output.

## Summary

| Value | plan | apply |
|-------|------|-------|
| `warn` | Shows drift warning | Skips the file |
| `overwrite` | Shows diff | Overwrites with declared content |
| `skip` | No output | No action |

## Runtime Override in Diff Viewer

During `gh infra apply`, you can press `d` at the confirmation prompt to open the interactive diff viewer. Inside the viewer, press `Tab` to cycle the `on_drift` setting for the selected file:

```
warn → overwrite → skip → warn
```

This override applies only to the current run — the YAML manifest is not modified. Use this to make one-off decisions without changing your configuration. For example, you might normally use `on_drift: warn` but override a specific file to `overwrite` after reviewing the diff.

See [apply command](../../commands/apply/#interactive-diff-viewer) for full keybindings.

## Interaction with `sync_mode: create_only`

`on_drift` does not apply to `create_only` files. Since `create_only` skips the file entirely once it exists, there is no content comparison and therefore no drift. Setting both on the same file is a validation error:

```yaml
# ✗ Error: on_drift cannot be set on a file with sync_mode "create_only"
files:
  - path: VERSION
    content: "0.1.0"
    sync_mode: create_only
    on_drift: warn        # ← validation error
```

See [Sync Mode](../sync-mode/#create_only) for details.

## Interaction with `sync_mode: mirror`

`on_drift` and `sync_mode: mirror` cannot be used on the **same file**. Mirror means "make the directory exactly match the source" — content drift is always resolved by overwriting, so a per-file `on_drift` would be contradictory:

```yaml
# ✗ Error: on_drift cannot be set on a file with sync_mode "mirror"
files:
  - path: .github/workflows
    source: ./templates/workflows/
    sync_mode: mirror
    on_drift: warn        # ← validation error
```

However, spec-level `on_drift` and mirror files **can coexist**. Mirror files ignore the spec-level setting and always overwrite, while non-mirror files use it:

```yaml
# ✓ Valid: spec-level on_drift + mirror on a different file
spec:
  on_drift: overwrite

  files:
    - path: .gitignore
      content: ...
      # uses spec-level on_drift: overwrite

    - path: .github/workflows
      source: ./templates/workflows/
      sync_mode: mirror    # always overwrites, ignores spec-level on_drift
```
