---
name: gh-infra
description: >
  Overview of gh-infra and command workflow (import, validate, plan, apply).
  Use when managing GitHub repository settings, labels, actions settings, rulesets,
  secrets, variables, or files declaratively via YAML manifests.
---

# gh-infra

gh-infra is a declarative GitHub infrastructure tool for repository settings and managed files.

Use this skill to choose the right resource kind, command flow, and operating pattern. Use the related skills for schema details.

Key characteristics:

- No state file. GitHub is the source of truth.
- Selective management. Omitted fields are left untouched.
- Four resource kinds: `Repository`, `RepositorySet`, `File`, `FileSet`.
- Supports both bootstrap import and reverse import into existing manifests.

## Use This Skill For

- Choosing between `Repository` / `RepositorySet` / `File` / `FileSet`
- Running `import`, `validate`, `plan`, and `apply`
- Picking a central-management vs self-managed repo layout
- Finding the right manifest skill for a concrete edit
- Routing `import --into` work to the dedicated skill

## Related Skills

| Task | Skill |
|------|-------|
| Write/edit `Repository` or `RepositorySet` YAML | `repository-manifest` |
| Write/edit `File` or `FileSet` YAML | `file-manifest` |
| Set up CI workflows and auth | `ci-cd` |
| Pull live GitHub state back into existing manifests | `import-into` |

## Resource Selection

Use:

- `Repository` for one repository's settings in one file
- `RepositorySet` for many repositories with shared defaults
- `File` for files in one repository
- `FileSet` for distributing shared files to many repositories

Every manifest starts with `apiVersion` and `kind`.

Single-repo resources:

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: <github-owner>
  name: <repo-name>
spec:
  # ...
```

Set resources:

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: <github-owner>    # no "name" field
defaults:                   # RepositorySet: shared defaults
  spec: { ... }
repositories:               # RepositorySet: per-repo entries
  - name: <repo-name>
    spec: { ... }
```

```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: <github-owner>    # no "name" field
spec:
  repositories: [...]       # FileSet: target repos
  files: [...]              # FileSet: files to distribute
```

A single YAML file can contain multiple `---`-separated documents. Anchors do not cross document boundaries.

## Command Workflow

Default workflow:

```text
import -> edit YAML -> validate -> plan -> apply
```

### import

Bootstrap a manifest from live GitHub state:

```bash
gh infra import <owner/repo>
```

### validate

Validate syntax and schema without contacting GitHub:

```bash
gh infra validate [path]
```

### plan

Show diff against live GitHub state:

```bash
gh infra plan [path]
```

Use `--ci` for drift-detection workflows.

### apply

Apply changes to GitHub:

```bash
gh infra apply [path]
```

Use `--auto-approve` in CI. `--force-secrets` re-sends all declared secrets.

## Path Behavior

For `validate`, `plan`, and `apply`:

- No argument or `.`: read `*.yaml` and `*.yml` in the current directory
- File path: read that file only
- Directory path: read top-level `*.yaml` and `*.yml` only
- Subdirectories are not scanned
- Unknown YAML kinds are skipped unless `--fail-on-unknown` is set

## Common Patterns

- Central management repo: keep org-wide manifests in `repos/` and `files/`
- Self-managed repo: keep one manifest inside the managed repository and auto-apply on merge

Read [references/patterns.md](./references/patterns.md) for layout guidance.

## Read Next

- Command details: [references/commands.md](./references/commands.md)
- Operating patterns: [references/patterns.md](./references/patterns.md)

## Example Multi-Doc File

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: my-org
  name: my-repo
spec:
  visibility: public
---
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
  via: push
```
