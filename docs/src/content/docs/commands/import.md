---
title: import
sidebar:
  order: 0
---

Export the current GitHub repository settings as YAML. If you already have local manifests, use `--into` to pull the current GitHub state back into those manifests and local source files.

```bash
gh infra import <owner/repo>
```

## Flags

| Flag | Description |
|------|-------------|
| `--into <path>` | Import GitHub state back into an existing manifest file or manifest directory |

## Export Repository Settings

Export the current repository settings as a complete `Repository` YAML manifest. This is the starting point when adopting gh-infra for an existing repository.

```bash
gh infra import <owner/repo>
```

### Examples

```bash
# Import and save to a file
gh infra import babarot/my-project > repos/my-project.yaml

# Import and review
gh infra import babarot/my-project
```

The output is a complete `Repository` YAML manifest reflecting the current state of the repository on GitHub.

If you want to save the exported YAML, redirect the output to a file:

```bash
gh infra import babarot/my-project > repos/my-project.yaml
```

## Import Into Existing Manifests

Use `--into` when you already have local manifests and want to pull the current GitHub state back into them.

```bash
gh infra import <owner/repo> --into=<path>
```

`<path>` can be:

- a manifest file
- a directory containing manifests

This is the reverse direction of `apply`:

- `apply` pushes desired local state to GitHub
- `import --into` pulls current GitHub state back into local manifests and source files

Before writing anything, gh-infra shows the planned changes and asks for confirmation.

## What Gets Updated

`import --into` can update the following local resources when they match the target repository:

- `Repository`
- `RepositorySet` entry specs
- `File`
- `FileSet`

For files, write-back behavior depends on how the file was declared:

| Manifest declaration | Local write-back target |
|---|---|
| inline `content` block | Update the inline YAML content block |
| `source: ./local-file` | Overwrite the local source file |
| `source: ./local-dir/` | Overwrite files under the local source directory |
| `source: github://...` | Not writable locally, so skipped |

For `RepositorySet`, `import --into` updates the matching `repositories[i].spec` entry. It does **not** automatically lift shared values back into `defaults.spec`.

## What Gets Skipped

Some files are intentionally not written during import:

- `reconcile: create_only`
- files that do not exist on GitHub
- remote sources such as `github://...`

You may also see warnings for entries that use:

- patches
- templates

In these cases, gh-infra still imports the GitHub content, but warns that the pulled content may be repository-specific or already include applied transformations.

## Examples

### Import repo settings and file content into one manifest

```bash
gh infra import babarot/gomi --into=./repos/gomi.yaml
```

### Import into all manifests under a directory

```bash
gh infra import babarot/gomi --into=./repos
```

## When to Use `import`

Use plain `import` when:

- you are bootstrapping gh-infra for an existing repository
- you want a fresh YAML manifest exported from GitHub

Use `import --into` when:

- you already manage a repo with gh-infra
- local manifests or templates have drifted from the real GitHub state
- you want to pull current file contents or repo settings back into your local source of truth
- you want to review the planned local write-back and confirm it interactively

## Limitations

- `RepositorySet` import updates `repositories[i].spec` only
- automatic reverse-merge into `defaults.spec` is not supported
- `github://` sources are read-only from the perspective of local write-back
- `import --into` requires an interactive terminal for confirmation
