# Write Modes

## `write`

Use when the local source of truth should move to match GitHub.

- inline content: rewrite the YAML content block
- local source file: rewrite that local file

## `patch`

Use when the drift is repository-specific and should not immediately rewrite a shared source.

This is the safest default for:

- shared local source files
- cases where the repo differs slightly from a common template
- existing `patches:`-based entries

## `skip`

Use when:

- the drift should not be imported yet
- you want to inspect manually first
- the file is `create_only` and should usually remain local-authoritative
- the file comes from a directory source that other FileSets share: it has no `patch` (below), so it is skipped by default; `Tab` to `write` rewrites the shared file

## Directory sources

A file expanded from a directory source (`path: policy, source: ./policy/`) has no `patch`: the patches of a directory entry apply to every file in it. Only `write` (rewrite the local file) and `skip`. With patches already on the directory entry, the file is hard-skipped (`expanded from directory source`).

Inline and patch write-back targets are the entry as written in the manifest (`spec.files[i]`, or the repository's `overrides[i]`), not positions after directory expansion.

An override cannot remove the patches of the `files` entry it overrides; when GitHub matches the content before those patches, the file is hard-skipped.

## Practical Rule

If changing the shared source would unintentionally affect other repositories, prefer `patch`.
