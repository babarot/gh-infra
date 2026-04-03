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
| `Tab` | Toggle apply/skip for the selected file |
| `d`/`u` | Scroll diff pane |
| `q`/`Esc` | Return to confirmation |

Repository setting changes (description, visibility, features, etc.) are shown in the terminal plan output, not in the diff viewer.

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
| Files using template variables (`vars:`) | Rendered content cannot be reverse-transformed to template |
| Files using patches (`patches:`) | Patched content cannot be split back to base + patch |
| Files from GitHub source (`source: github://...`) | No local file to write back to |
| Secrets | GitHub API does not return secret values; local values are preserved |

Skipped files are shown in the plan output with a warning icon and the skip reason displayed dimmed.

### Shared Source Files

When a source file is shared across multiple repositories in a FileSet, importing from one repository updates the shared source. This is expected: the source is the single point of truth, so pulling drift from one repo propagates to all. A warning is shown in the plan output for visibility.
