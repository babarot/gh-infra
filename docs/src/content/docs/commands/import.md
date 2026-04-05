---
title: import
sidebar:
  order: 0
---

Export existing repository settings as YAML. Useful for bootstrapping gh-infra configuration from an existing repository.

```bash
gh infra import <owner/repo> [owner/repo ...]
```

## Examples

```bash
# Import and save to a file
gh infra import babarot/my-project > repos/my-project.yaml

# Import multiple repositories
gh infra import babarot/my-project babarot/my-cli

# Import and review
gh infra import babarot/my-project
```

The output is a complete `Repository` YAML manifest reflecting the current state of the repository on GitHub.

## `--into`: Pull GitHub State into Local Manifests

With `--into`, import works in the reverse direction of `plan`/`apply`: it fetches the current GitHub state and updates your existing local YAML manifests to match.

```bash
gh infra import <owner/repo> --into=<path>
```

The path can be a single YAML file or a directory containing manifests.

### How It Works

1. **Parse** local manifests at the given path
2. **Match** each `owner/repo` argument to resources in the manifests (Repository, RepositorySet, FileSet)
3. **Fetch** the current state from GitHub
4. **Diff** local vs GitHub, field by field
5. **Display** the plan (repo setting changes + file changes with diff stats)
6. **Confirm** with interactive diff viewer (for file changes) or simple prompt (repo-only changes)
7. **Write** approved changes to local files

### Examples

```bash
# Pull GitHub state into a specific manifest file
gh infra import babarot/my-project --into=repos/my-project.yaml

# Pull from a directory of manifests
gh infra import babarot/my-project --into=repos/

# Import multiple repositories at once
gh infra import babarot/my-project babarot/my-cli --into=repos/
```

### Interactive Diff Viewer

After the plan is displayed, the confirmation prompt offers three options:

```
> Apply import changes? (yes / no / diff)
```

Press `d` to open a full-screen diff viewer for file-level changes:

| Key | Action |
|-----|--------|
| `↑`/`↓` or `j`/`k` | Select file |
| `Tab` | Cycle `write` / `patch` / `skip` for the selected file |
| `d`/`u` | Scroll diff pane |
| `q`/`Esc` | Return to confirmation |

Repository setting changes (description, visibility, features, etc.) are shown in the terminal plan output, not in the diff viewer.

### `write` / `patch` / `skip`

The import diff viewer does not just let you approve or reject a file change. For each file, you can choose how the change should be written back:

- `write`
  - update the file's normal write-back target
  - inline entries update the manifest `content: |` block
  - source-backed entries update the local source file
- `patch`
  - store the drift as a manifest patch under `patches:`
- `skip`
  - do not apply that file change in this import run

The default action depends on the file shape:

| File shape | Default | Allowed |
|-----------|---------|---------|
| Inline content | `write` | `write`, `skip` |
| Local source (single-use) | `write` | `write`, `patch`, `skip` |
| Local source shared by multiple repos | `patch` | `write`, `patch`, `skip` |
| Existing `patches:` entry | `patch` | `write`, `patch`, `skip` |
| Template-based file (`vars:` / templating) | skipped in plan | not shown in viewer |
| `github://` source | skipped in plan | not shown in viewer |

This is especially useful for shared source files:

- the safe default is `patch`, so one repo's drift does not immediately rewrite the shared source
- but if you intentionally want to update the shared source/template itself, switch that entry to `write`

### What Gets Imported

| Resource | Behavior |
|----------|----------|
| Repository settings | Field-by-field comparison and YAML patch |
| RepositorySet entries | Minimal override reconstruction (preserves defaults/override separation) |
| Files with local source (`source: ./path`) | Local file overwritten with GitHub content |
| Files with inline content (`content: \|`) | YAML content block updated in-place |
| Files with `reconcile: create_only` | Imported (updates the local master template for future repos) |

### What Gets Skipped

| Source | Reason |
|--------|--------|
| Templated files (`vars:`, `{{ ... }}`, `<% ... %>`, etc.) | Rendered content cannot be safely reverse-transformed back into the original template |
| Files from GitHub source (`source: github://...`) | No local file to write back to |
| Secrets | GitHub API does not return secret values; local values are preserved |

Skipped files are shown in the plan output with a warning icon and the skip reason displayed dimmed.

### Shared Source Files

When a source file is shared across multiple repositories in a FileSet, the default import action is `patch`, not `write`.

This means:

- by default, drift from one repository is captured in manifest patches
- the shared source file is left unchanged
- other repositories are not affected immediately

If you intentionally want to make the shared source authoritative and propagate that change, switch the entry to `write` in the diff viewer.
