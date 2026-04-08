# Write Modes

## `write`

Use when the local source of truth should move to match GitHub.

- inline content: rewrite the YAML content block
- local source file: rewrite that local file

## `patch`

Use when the drift is repository-specific and should not immediately rewrite the source.

Available for all file types (source-backed and inline-backed).

This is the safest default for:

- shared local source files
- cases where the repo differs slightly from a common template
- existing `patches:`-based entries
- inline content where you want to preserve the original `content:` block and store the drift separately

## `skip`

Use when:

- the drift should not be imported yet
- you want to inspect manually first
- the file is `create_only` and should usually remain local-authoritative

## Practical Rule

If changing the shared source would unintentionally affect other repositories, prefer `patch`.
