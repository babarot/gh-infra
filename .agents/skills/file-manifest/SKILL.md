---
name: file-manifest
description: >
  Complete YAML schema reference for File and FileSet resources.
  Use when writing manifests to manage files (CODEOWNERS, LICENSE, CI workflows, etc.)
  in one or more repositories, including templating, patches, reconcile modes,
  delivery method, and import-into behavior.
---

# File Manifest Reference

Use this skill for file-distribution manifests. Keep the main body small and open references only when the request needs them.

## Core Rules

- `File` is parsed as a one-repo `FileSet` internally, but you still author the simpler shape
- Each file entry requires `path` and exactly one of `content` or `source`
- `via` defaults to `push`
- `reconcile` defaults to `patch`
- `patches` are applied after template expansion

## File

```yaml
apiVersion: gh-infra/v1
kind: File
metadata:
  owner: my-org
  name: my-repo
spec:
  files:
    - path: .github/CODEOWNERS
      content: |
        * @username
    - path: LICENSE
      source: ./templates/LICENSE
  via: push
  commit_message: "ci: sync managed files"
```

Key fields:

- `files`: required
- `via`: `push` or `pull_request`
- `commit_message`, `branch`, `pr_title`, `pr_body`: delivery controls
- `files[].vars`: template variables
- `files[].patches`: unified diff patches
- `files[].reconcile`: `patch`, `mirror`, `create_only`

Read these references as needed:

- Sources and templating: [references/sources-and-templating.md](./references/sources-and-templating.md)
- Reconcile and delivery: [references/reconcile-and-delivery.md](./references/reconcile-and-delivery.md)
- Patches: [references/patches.md](./references/patches.md)
- FileSet and overrides: [references/fileset.md](./references/fileset.md)
- Import-into behavior: use the dedicated `import-into` skill

## FileSet

Use `FileSet` to distribute shared files to many repositories.

```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: my-org            # no "name" field

spec:
  repositories:
    - gomi                # simple string
    - enhancd
    - name: gh-infra      # struct with overrides
      overrides:
        - path: .github/CODEOWNERS
          content: |
            * @username @co-maintainer

  files:
    - path: .github/CODEOWNERS
      content: |
        * @username

    - path: LICENSE
      source: ./templates/LICENSE

  via: push
```

Each repository entry is either:

- a simple repo name string
- an object with `name` and optional `overrides`

Overrides replace the matching base entry by `path` for that repository only.

## High-Value Gotchas

- Prefer patch files over inline patch blocks for files with tabs
- Patch context must match template-expanded content, not raw template source
- `github://` sources are import hard-skips because there is no local write target
- Shared local source files in `FileSet` often default to `patch` in `import --into`
- `create_only` affects apply behavior, but import can still update the local master template

## Verification

```bash
gh infra validate <path>
gh infra plan <path>
```
