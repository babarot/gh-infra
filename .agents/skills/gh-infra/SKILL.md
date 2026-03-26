---
name: gh-infra
description: >
  Overview of gh-infra and complete command reference (import, validate, plan, apply).
  Use when managing GitHub repository settings, branch protection, rulesets, secrets,
  variables, or files declaratively via YAML manifests.
---

# gh-infra

gh-infra is a declarative GitHub infrastructure management tool. Define repository settings in YAML, then use a Terraform-like `plan` → `apply` workflow to sync them to GitHub.

Key characteristics:

- **No state file** — GitHub is the source of truth. gh-infra fetches live state on every run.
- **Selective management** — only fields declared in YAML are managed. Omitted fields are left untouched.
- **Four resource kinds**: `Repository`, `RepositorySet`, `File`, `FileSet`.

## Related Skills

| Task | Skill to use |
|------|-------------|
| Write/edit `Repository` or `RepositorySet` YAML | **repository-manifest** |
| Write/edit `File` or `FileSet` YAML | **file-manifest** |
| Set up GitHub Actions workflows (`--ci`, `--auto-approve`) | **ci-cd** |

## YAML Document Structure

Every manifest starts with `apiVersion` and `kind`. The structure differs between single-repo and set kinds:

Single-repo kinds (`Repository`, `File`):

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: <github-owner>
  name: <repo-name>
spec:
  # ...
```

Set kinds (`RepositorySet`, `FileSet`):

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

A single YAML file can contain multiple documents separated by `---`. This allows mixing different resource kinds in one file:

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: babarot
  name: gomi
spec:
  visibility: public
---
apiVersion: gh-infra/v1
kind: File
metadata:
  owner: babarot
  name: gomi
spec:
  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot
  via: push
```

Note: YAML anchors do not work across document boundaries (`---`). Each document has its own scope.

## Commands

### import

Export an existing repository's settings as a complete `Repository` YAML manifest.

```bash
gh infra import <owner/repo>
```

Example:

```bash
gh infra import babarot/my-project > repos/my-project.yaml
```

Use this as the starting point when adopting gh-infra for an existing repo — import, review, edit, then plan/apply.

### validate

Check YAML syntax and schema without contacting GitHub.

```bash
gh infra validate [path]
```

| Argument | Behavior |
|----------|----------|
| *(none)* or `.` | All `*.yaml` in the current directory |
| File | That file only |
| Directory | All `*.yaml` directly under it (subdirectories are ignored) |

Flags:

| Flag | Description |
|------|-------------|
| `--fail-on-unknown` | Error on YAML files with unknown Kind (default: silently skip) |

Exits 0 if all files are valid.

### plan

Show diff between YAML and current GitHub state. **No mutations are made.**

```bash
gh infra plan [path]
```

Path behavior is the same as `validate`. Non-gh-infra YAML files are silently skipped.

Flags:

| Flag | Description |
|------|-------------|
| `-r, --repo <owner/repo>` | Target a specific repository |
| `--ci` | Exit with code 1 if changes detected (for CI drift detection) |
| `--fail-on-unknown` | Error on YAML files with unknown Kind |

Examples:

```bash
gh infra plan ./repos/
gh infra plan ./repos/gomi.yaml
gh infra plan ./repos/ --repo babarot/gomi
gh infra plan ./repos/ --ci
```

### apply

Apply changes to GitHub. Requires interactive confirmation by default.

```bash
gh infra apply [path]
```

Path behavior is the same as `validate`.

Flags:

| Flag | Description |
|------|-------------|
| `-r, --repo <owner/repo>` | Target a specific repository |
| `--auto-approve` | Skip confirmation prompt (required for CI) |
| `--force-secrets` | Re-set all secrets even if they already exist |
| `--fail-on-unknown` | Error on YAML files with unknown Kind |

Interactive diff viewer (at confirmation prompt):

| Key | Action |
|-----|--------|
| `d` | Open full-screen diff viewer |
| `↑`/`↓` or `j`/`k` | Select file |
| `Tab` | Toggle apply/skip for the selected file |
| `d`/`u` | Scroll diff pane |
| `q`/`Esc` | Return to confirmation |

Toggling a file to "skip" is a runtime-only decision — the YAML manifest is not modified.

Examples:

```bash
gh infra apply ./repos/
gh infra apply ./repos/ --auto-approve
gh infra apply ./repos/ --force-secrets
gh infra apply ./repos/ --repo babarot/gomi
```

## Recommended Workflow

```
import → edit YAML → validate → plan → apply
```

1. `gh infra import owner/repo > repos/repo.yaml` — capture current state
2. Edit the YAML to declare the desired state
3. `gh infra validate` — check syntax
4. `gh infra plan` — review what would change
5. `gh infra apply` — apply the changes

## Global Flags

| Flag | Description |
|------|-------------|
| `-V, --verbose` | Show gh command execution details |
| `--log-level <level>` | Set log level: trace, debug, info, warn, error |
