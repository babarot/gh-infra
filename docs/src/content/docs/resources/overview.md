---
title: Overview
sidebar:
  order: 0
---

gh-infra manages GitHub infrastructure through four resource kinds:

| Kind | Scope | Description |
|------|-------|-------------|
| [Repository](../repository/) | 1 repo | Settings, features, branch protection, rulesets, secrets, variables |
| [RepositorySet](../repository-set/) | N repos | Shared defaults across multiple repositories |
| [File](../file/) | 1 repo | Manage files (CODEOWNERS, LICENSE, workflows, etc.) in a single repo |
| [FileSet](../fileset/) | N repos | Distribute files across multiple repositories |

## Common Structure

All resources share the same top-level structure:

```yaml
apiVersion: gh-infra/v1
kind: <Repository | RepositorySet | File | FileSet>
metadata:
  # Repository: name + owner
  # RepositorySet, FileSet: owner
  owner: <github-owner>

spec:
  # Resource-specific fields
```

- **Repository** and **File** use `metadata.name` (repo name) and `metadata.owner` (GitHub owner) to identify a single repo.
- **RepositorySet** and **FileSet** use `metadata.owner` to scope all entries to one owner. Individual repositories are listed in the body.

## File Organization

You can organize YAML files however you like. gh-infra accepts a file or directory path:

```bash
gh infra plan ./repos/           # All YAML files in the directory
gh infra plan ./repos/gomi.yaml  # A single file
```

Multiple resource kinds can coexist in the same directory. gh-infra processes each file based on its `kind`.
