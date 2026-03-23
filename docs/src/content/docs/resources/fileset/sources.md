---
title: File Sources
---

Each file entry in a FileSet specifies **where the content comes from** via `content` (inline) or `source` (external). There are four source types:

## Inline Content

Write the file content directly in YAML. Best for short files like CODEOWNERS or security policies.

```yaml
files:
  - path: .github/CODEOWNERS
    content: |
      * @babarot
```

## Local File

Read content from a file on disk. Paths are resolved relative to the YAML file's location.

```yaml
files:
  - path: LICENSE
    source: ./templates/LICENSE
```

## Local Directory

Sync an entire directory. A trailing slash on the source indicates a directory. All files under it are expanded with paths relative to `path`.

```yaml
files:
  - path: .github/workflows
    source: ./templates/workflows/
```

For example, if `./templates/workflows/` contains `ci.yaml` and `release.yaml`, this creates `.github/workflows/ci.yaml` and `.github/workflows/release.yaml` in the target repos.

## GitHub Repository

Pull files directly from another GitHub repository using the `github://` scheme. This is useful when a central "shared-config" repo holds your templates — no need to clone it locally.

The format is:

```
github://<owner>/<repo>/<path>          # file (default branch)
github://<owner>/<repo>/<path>/         # directory (trailing slash)
github://<owner>/<repo>/<path>@<ref>    # file pinned to tag/branch
```

Authentication is handled by `gh auth`.

### Single file

```yaml
files:
  - path: .goreleaser.yaml
    source: github://myorg/shared-config/.goreleaser.yaml
```

### Directory

A trailing slash fetches **all files** in the directory, including subdirectories (recursively).

```yaml
files:
  # Sync all CI workflows from the shared-config repo
  - path: .github/workflows
    source: github://myorg/shared-config/workflows/
```

For example, if `workflows/` in the source repo contains:

```
workflows/
├── ci.yaml
├── release.yaml
└── checks/
    └── lint.yaml
```

This creates `.github/workflows/ci.yaml`, `.github/workflows/release.yaml`, and `.github/workflows/checks/lint.yaml` in each target repo.

### Pinning to a version

By default, `github://` fetches from the default branch. Append `@ref` to pin to a specific tag, branch, or commit:

```yaml
files:
  # Pin to a release tag
  - path: .github/workflows/ci.yaml
    source: github://myorg/shared-config/workflows/ci.yaml@v1.0.0

  # Pin to a branch
  - path: .github/CODEOWNERS
    source: github://myorg/shared-config/CODEOWNERS@main

  # Pin to a commit SHA
  - path: Makefile
    source: github://myorg/shared-config/Makefile@a1b2c3d
```

:::tip
Pin to a tag (e.g., `@v1.0.0`) in production to avoid unexpected changes when the source repo is updated. Use an unpinned reference during development to always get the latest.
:::

### Combining with other source types

You can mix `github://` sources with local and inline sources in the same FileSet:

```yaml
files:
  # From GitHub
  - path: .github/workflows
    source: github://myorg/shared-config/workflows/

  # From local file
  - path: LICENSE
    source: ./templates/LICENSE

  # Inline
  - path: .github/CODEOWNERS
    content: |
      * @babarot
```

## Keeping Content DRY with YAML Anchors

Use YAML anchors to avoid duplicating inline content within a single file:

```yaml
_templates:
  codeowners: &codeowners |
    * @babarot

  license: &license |
    MIT License
    Copyright (c) 2025 babarot

spec:
  files:
    - path: .github/CODEOWNERS
      content: *codeowners
    - path: LICENSE
      content: *license
```

:::note
YAML anchors work within a single file only. They cannot cross file boundaries — this is a YAML spec limitation. If you need to share content across files, use `source` to reference a local file instead.
:::
