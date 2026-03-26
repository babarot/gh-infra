---
title: Overview
sidebar:
  order: 0
---

gh-infra manages GitHub infrastructure through four resource kinds, organized in two pairs:

| | 1 repo | N repos |
|---|---|---|
| **Repository settings** | [Repository](../repository/) | [RepositorySet](../repository-set/) |
| **File management** | [File](../file/) | [FileSet](../fileset/) |

The single-repo resource (`Repository`, `File`) manages one repository in detail. The set resource (`RepositorySet`, `FileSet`) applies shared configuration across multiple repositories — each entry can override specific values.

## Common Structure

All resources share the same top-level structure:

```yaml
apiVersion: gh-infra/v1
kind: <Repository | RepositorySet | File | FileSet>
metadata:
  owner: <github-owner>
  name: <repo-name>       # single-repo resources only

spec:
  # Resource-specific fields
```

### Metadata

| Resource | `metadata.owner` | `metadata.name` | Identifies |
|---|---|---|---|
| **Repository** | required | required | A single repo (`owner/name`) |
| **File** | required | required | A single repo (`owner/name`) |
| **RepositorySet** | required | — | All repos listed in `repositories` |
| **FileSet** | required | — | All repos listed in `repositories` |

## File Organization

You can organize YAML files however you like. gh-infra accepts a file or directory path:

```bash
gh infra plan ./repos/           # All YAML files in the directory
gh infra plan ./repos/gomi.yaml  # A single file
```

Multiple resource kinds can coexist in the same directory. gh-infra processes each file based on its `kind`.

### Multi-Document YAML

A single YAML file can contain multiple documents separated by `---`. This lets you group related resources together without creating separate files:

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-app
  owner: my-org
spec:
  description: "Application repository"
  visibility: public
---
apiVersion: gh-infra/v1
kind: File
metadata:
  name: my-app
  owner: my-org
spec:
  files:
    - path: .github/CODEOWNERS
      content: |
        * @my-org/platform
  via: push
```

You can mix any resource kinds within a single file. Each document is parsed independently, so they do not share YAML anchors or other state.

:::tip
Multi-document YAML works well for pairing a Repository with its File resource — everything about a single repo lives in one place.
:::
