---
name: file-manifest
description: >
  Complete YAML schema reference for File and FileSet resources.
  Use when writing manifests to manage files (CODEOWNERS, LICENSE, CI workflows, etc.)
  in one or more repositories.
---

# File Manifest Reference

## File (kind: File)

Manages **files** in a **single** repository.

```yaml
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

    - path: LICENSE
      source: ./templates/LICENSE

  via: push
  commit_message: "ci: sync managed files"
```

### Spec Fields

| Field | Default | Description |
|-------|---------|-------------|
| `files` | *(required)* | List of file entries |
| `via` | `push` | Delivery method: `push` or `pull_request` |
| `commit_message` | auto | Custom commit message |
| `branch` | auto | Branch name (`pull_request` only) |
| `pr_title` | value of `commit_message` | PR title (`pull_request` only) |
| `pr_body` | auto | PR body, supports Markdown (`pull_request` only) |

### File Entry Fields

Each entry in `files` requires `path` and either `content` or `source`:

| Field | Description |
|-------|-------------|
| `path` | **Required.** Target path in the repository |
| `content` | Inline content (mutually exclusive with `source`) |
| `source` | External source path (mutually exclusive with `content`) |
| `vars` | Custom template variables (map of string → string) |
| `reconcile` | Reconcile mode: `patch` (default), `mirror`, or `create_only` |


## File Sources

### Inline Content

```yaml
files:
  - path: .github/CODEOWNERS
    content: |
      * @babarot
```

### Local File

Path relative to the YAML file's location:

```yaml
files:
  - path: LICENSE
    source: ./templates/LICENSE
```

### Local Directory

Trailing slash indicates a directory. All files are expanded recursively:

```yaml
files:
  - path: .github/workflows
    source: ./templates/workflows/
```

If `./templates/workflows/` contains `ci.yaml` and `release.yaml`, this creates `.github/workflows/ci.yaml` and `.github/workflows/release.yaml`.

### GitHub Repository

Pull files from another GitHub repository using `github://` scheme:

```
github://<owner>/<repo>/<path>          # file (default branch)
github://<owner>/<repo>/<path>/         # directory (trailing slash)
github://<owner>/<repo>/<path>@<ref>    # pinned to tag/branch/SHA
```

Examples:

```yaml
files:
  - path: .goreleaser.yaml
    source: github://myorg/shared-config/.goreleaser.yaml

  - path: .github/workflows
    source: github://myorg/shared-config/workflows/

  - path: Makefile
    source: github://myorg/shared-config/Makefile@v1.0.0
```

Pin to a tag (e.g., `@v1.0.0`) in production. Use unpinned during development.

## Templating

gh-infra uses `<% %>` delimiters — no conflict with `${{ }}` (GitHub Actions), `{{ }}` (Go/Helm), or any other template system. No escaping needed.

### Built-in Variables

| Variable | Example (`babarot/gomi`) |
|----------|--------------------------|
| `<% .Repo.Name %>` | `gomi` |
| `<% .Repo.Owner %>` | `babarot` |
| `<% .Repo.FullName %>` | `babarot/gomi` |

### Custom Variables

```yaml
files:
  - path: .github/workflows/release.yaml
    source: ./templates/release.yaml
    vars:
      binary_name: "<% .Repo.Name %>"
      docker_image: "ghcr.io/<% .Repo.FullName %>"
```

In the template:

```yaml
- run: docker build -t <% .Vars.docker_image %> .
- run: ./<% .Vars.binary_name %> --version
```

Two-pass expansion:
1. `vars` values are expanded (can reference `.Repo` only)
2. File content is expanded (can reference both `.Repo` and `.Vars`)

Undefined variable references cause an error — no silent empty strings.

## Reconcile Mode

Controls how each file entry is managed in the target repository.

| Mode | Creates | Updates | Deletes orphans |
|------|---------|---------|-----------------|
| `patch` (default) | Yes | Yes | No |
| `mirror` | Yes | Yes | Yes |
| `create_only` | Yes | No | No |

### patch (default)

Add/update declared files. Other files in the directory are untouched.

```yaml
files:
  - path: .github/CODEOWNERS
    content: "* @platform-team"
    # reconcile: patch  ← default
```

### mirror

Make the target directory an exact copy of the source. Orphan files are deleted.

```yaml
files:
  - path: .github/workflows
    source: ./templates/workflows/
    reconcile: mirror
```

Mirror only affects the directory specified by `path`. Files outside it are never touched.

### create_only

Create once, never update. For seed files that repos manage independently.

```yaml
files:
  - path: VERSION
    content: "0.1.0"
    reconcile: create_only
```

Different files can use different modes in the same manifest.

## Delivery Method (via)

### push (default)

Direct commit to the default branch. All file changes in one atomic commit.

```yaml
spec:
  via: push
  commit_message: "ci: sync shared files"
```

### pull_request

Create a branch + PR for review before merging.

```yaml
spec:
  via: pull_request
  commit_message: "ci: sync shared files"
  branch: gh-infra/sync-shared
  pr_title: "Sync shared files"
  pr_body: |
    Automated file sync by gh-infra.
```

If a PR already exists for the branch, gh-infra updates it.

## YAML Anchors

Avoid duplicating inline content within a single file:

```yaml
_templates:
  codeowners: &codeowners |
    * @babarot

spec:
  files:
    - path: .github/CODEOWNERS
      content: *codeowners
```

Anchors work within a single file only (YAML spec limitation).

## FileSet (kind: FileSet)

Distributes **files** to **multiple** repositories.

```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot          # no "name" field

spec:
  repositories:
    - gomi                # simple string
    - enhancd
    - name: gh-infra      # struct with overrides
      overrides:
        - path: .github/CODEOWNERS
          content: |
            * @babarot @co-maintainer

  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot

    - path: LICENSE
      source: ./templates/LICENSE

  via: push
```

### Repositories

Each entry is either a simple string (repo name) or a struct with `name` and `overrides`.

All repos are under the same `metadata.owner`.

### Per-Repo Overrides

An override replaces the matching file entry (matched by `path`) for that repository only:

```yaml
spec:
  repositories:
    - gomi
    - name: special-repo
      overrides:
        - path: .github/CODEOWNERS
          content: |
            * @babarot @special-team

  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot
```

- `gomi` gets `* @babarot`
- `special-repo` gets `* @babarot @special-team`

Overrides inherit `vars` from the original file entry if not specified. Files without a matching override use the default from `files`.

### Complete Example

```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot

spec:
  repositories:
    - gomi
    - enhancd
    - name: gh-infra
      overrides:
        - path: Makefile
          vars:
            binary_name: "gh-infra"

  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot

    - path: .github/workflows/ci.yaml
      source: ./templates/ci.yaml

    - path: Makefile
      source: ./templates/Makefile
      vars:
        binary_name: "<% .Repo.Name %>"

    - path: README.md
      source: ./templates/README.md
      reconcile: create_only

  via: pull_request
  pr_title: "chore: sync shared files"
  branch: gh-infra/sync
```

After writing a manifest, always run:

```bash
gh infra validate <path>
gh infra plan <path>
```
