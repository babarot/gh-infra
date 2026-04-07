---
name: import-into
description: >
  Use when pulling live GitHub state back into existing gh-infra manifests with
  `gh infra import --into`, especially for write/patch/skip decisions, shared
  file sources, template-backed files, and import safety rules.
---

# Import Into

Use this skill for `gh infra import --into=...`.

This workflow is different from normal `import`, `plan`, and `apply`:

- it updates local manifests from live GitHub state
- it may rewrite YAML, local source files, or manifest patches
- it has file-level write modes and skip safety rules

## When To Use

Use this skill when the user wants to:

- pull GitHub state back into existing manifests
- reconcile drift from GitHub UI into local YAML
- decide between `write`, `patch`, and `skip`
- understand why a file was skipped during import
- update shared templates safely after repository-side drift

## Command

```bash
gh infra import <owner/repo> [owner/repo ...] --into=<path>
```

`<path>` may be a file or a directory of manifests.

## How It Works

1. Parse local manifests
2. Match target repositories against `Repository`, `RepositorySet`, and `FileSet`
3. Fetch live state from GitHub
4. Diff local vs remote
5. Show plan output
6. For file changes, open the interactive diff viewer
7. Write approved local changes

## File Write Modes

- `write`: update the normal local target
- `patch`: store the drift under `patches:`
- `skip`: ignore for this run

Typical defaults:

- inline content: `write`
- single-use local source: `write`
- shared local source: `patch`
- existing `patches:` entry: `patch`
- many `create_only` entries: `skip`

## Safety Rules

There are two skip modes:

- default skip: safe enough to import, but skipped by default
- hard skip: cannot be written back safely

Hard-skip cases include:

- `github://` sources
- template-backed files whose placeholders cannot be reverse-mapped safely

## Template-Backed Files

For templates such as `<% .Repo.Name %>` and `<% .Repo.FullName %>`, gh-infra compares:

1. rendered local template
2. remote GitHub file

It then tries to reconstruct updated local template source while preserving placeholders.

Use `write` only when you want to update the underlying shared source. Use `patch` when drift should stay repository-specific.

## What Gets Updated

- repository settings: patched into YAML
- inline file content: written back into the manifest
- local `source: ./...` files: written back to the source file
- shared local source files: often better stored as patches

## What Does Not Get Imported

- secrets: GitHub does not return secret values
- `github://` sources: no local write target
- template-backed files that cannot be reverse-mapped safely

## Read Next

- Examples and detailed behavior: [references/behavior.md](./references/behavior.md)
- Diff-viewer decisions: [references/write-modes.md](./references/write-modes.md)
- Template safety: [references/template-safety.md](./references/template-safety.md)
