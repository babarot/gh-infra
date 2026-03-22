# gh-infra

[![Tests](https://github.com/babarot/gh-infra/actions/workflows/build.yaml/badge.svg)](https://github.com/babarot/gh-infra/actions/workflows/build.yaml)
[![Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/babarot/c8e4b1de0846824230d86cb2d86f38cf/raw/gh-infra-coverage.json)](https://github.com/babarot/gh-infra/actions/workflows/build.yaml)

Declarative GitHub infrastructure management via YAML. Like Terraform, but for GitHub — no state file required.

```
gh infra plan    # Show what would change
gh infra apply   # Apply the changes
```

## Why

The [Terraform GitHub Provider](https://registry.terraform.io/providers/integrations/github/latest/docs) covers most GitHub-as-Code use cases, but it's overkill for personal or small-team use — provider installation, HCL, state files, and state locking add real overhead before you can change a single setting.

gh-infra takes a different approach:

- **YAML instead of HCL.** Declare what your repos should look like in plain YAML.
- **No state file.** GitHub itself is the source of truth. Every `plan` fetches the live state and diffs directly — there's nothing to store, lock, or lose.
- **`plan` before `apply`.** See exactly what will change before it happens. Most alternatives (Probot Settings, GHaC) apply immediately with no preview.
- **One file, many repos.** A single `RepositorySet` can enforce consistent settings across dozens of repositories. No more clicking through the UI one repo at a time.
- **Just `gh` and a token.** No GitHub App, no server, no extra infrastructure. If you can run `gh`, you can run `gh infra`.

## How It Works

```mermaid
sequenceDiagram
    participant Y as YAML files
    participant C as gh-infra
    participant G as GitHub API

    rect rgba(128, 128, 128, 0.1)
    note right of Y: plan
    Y->>C: Parse desired state
    C->>G: Fetch current state (gh repo view, gh api)
    G-->>C: Current settings, protection, secrets...
    C->>C: Diff desired vs current
    C-->>Y: Show changes
    end

    rect rgba(128, 128, 128, 0.1)
    note right of Y: apply
    C->>G: gh repo edit, gh api PUT, gh secret set...
    G-->>C: OK
    C-->>Y: Applied ✓
    end
```

1. **Parse** YAML into Go structs
2. **Fetch** current state from GitHub API (`gh repo view --json`, `gh api`)
3. **Diff** desired vs current to produce a change set
4. **Apply** changes via `gh repo edit`, `gh api`, `gh secret set`, etc.

No state file needed — GitHub itself is the source of truth. Every `plan` fetches the live state and compares directly.

## Install

```bash
# As a gh extension
gh extension install babarot/gh-infra

# Or with Homebrew
brew install babarot/tap/gh-infra

# Or build from source
go install github.com/babarot/gh-infra/cmd/gh-infra@latest
```

## Quick Start

### 1. Import an existing repository

```bash
gh infra import babarot/my-project > repos/my-project.yaml
```

### 2. Edit the YAML to your desired state

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-project
  owner: babarot

spec:
  description: "My awesome project"
  visibility: public
  topics:
    - go
    - cli
  merge_strategy:
    allow_squash_merge: true
    allow_merge_commit: false
    allow_rebase_merge: false
    auto_delete_head_branches: true
```

### 3. Plan and apply

```bash
gh infra plan ./repos/
gh infra apply ./repos/
```

## Commands

| Command | Description |
|---------|-------------|
| `plan [path]` | Show diff between YAML and current GitHub state |
| `apply [path]` | Apply changes (with confirmation prompt) |
| `import <owner/repo>` | Export existing repo settings as YAML |
| `validate [path]` | Check YAML syntax and schema |

### Flags

```
Global:
  -V, --verbose             Show gh command execution details (shorthand for --log-level=debug)
      --log-level <level>   Log level: trace, debug, info, warn, error

plan:
  -r, --repo <owner/repo>   Target a specific repository
      --ci                   Exit with code 1 if changes detected

apply:
  -r, --repo <owner/repo>   Target a specific repository
      --auto-approve         Skip confirmation prompt
      --force-secrets        Re-set all secrets (even existing ones)
```

## YAML DSL Reference

### Available Kinds

| Kind | Scope | Description |
|------|-------|-------------|
| `Repository` | 1 repo | Manage a single repository's settings, features, branch protection, secrets, and variables |
| `RepositorySet` | N repos | Manage multiple repositories with shared defaults. Per-repo overrides supported |
| `FileSet` | N repos | Distribute files to repositories. Atomic commits via Git Data API, with optional PR strategy |

All resources use `apiVersion: gh-infra/v1` and are identified by `kind`.

### Repository

Manages a single GitHub repository's settings.

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: my-project           # Repository name (required)
  owner: babarot             # GitHub user or org (required)
  managed_by: self           # Optional: skip in central management mode

spec:
  # Basic settings
  description: "My awesome project"
  homepage: "https://example.com"
  visibility: public          # public | private | internal
  archived: false              # true to archive (read-only)

  topics:
    - go
    - cli
    - github

  # Features
  features:
    issues: true
    projects: false
    wiki: false
    discussions: false

  # Merge strategy
  merge_strategy:
    allow_merge_commit: false
    allow_squash_merge: true
    allow_rebase_merge: false
    auto_delete_head_branches: true
    squash_merge_commit_title: PR_TITLE      # PR_TITLE | COMMIT_OR_PR_TITLE
    squash_merge_commit_message: COMMIT_MESSAGES  # COMMIT_MESSAGES | PR_BODY | BLANK
    merge_commit_title: MERGE_MESSAGE        # MERGE_MESSAGE | PR_TITLE
    merge_commit_message: PR_TITLE           # PR_TITLE | PR_BODY | BLANK

  # Branch protection
  branch_protection:
    - pattern: main
      required_reviews: 1
      dismiss_stale_reviews: true
      require_code_owner_reviews: false
      require_status_checks:
        strict: true
        contexts:
          - "ci / test"
          - "ci / lint"
      enforce_admins: false
      allow_force_pushes: false
      allow_deletions: false

    - pattern: "release/*"
      required_reviews: 2
      allow_force_pushes: false

  # Repository secrets
  # Values use ${ENV_*} to reference environment variables.
  # Secrets are opaque — plan detects new ones but can't compare existing values.
  # Use `apply --force-secrets` to always re-set them.
  secrets:
    - name: DEPLOY_TOKEN
      value: "${ENV_DEPLOY_TOKEN}"
    - name: SLACK_WEBHOOK
      value: "${ENV_SLACK_WEBHOOK}"

  # Repository variables
  variables:
    - name: APP_ENV
      value: production
    - name: REGION
      value: us-central1
```

### RepositorySet

Manages multiple repositories with shared defaults. Per-repo values override defaults.

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: babarot

# Shared defaults
defaults:
  spec:
    visibility: public
    features:
      issues: true
      wiki: false
    merge_strategy:
      allow_squash_merge: true
      auto_delete_head_branches: true
    branch_protection:
      - pattern: main
        required_reviews: 1

# Individual repositories (override defaults as needed)
repositories:
  - name: gomi
    spec:
      description: "Trash CLI: a safe alternative to rm"
      topics: [go, cli, trash]
      features:
        discussions: true    # override default

  - name: enhancd
    spec:
      description: "A next-generation cd command with an interactive filter"
      topics: [zsh, shell, cd, fzf]

  - name: oksskolten
    spec:
      description: "The AI-native RSS reader"
      topics: [rss, self-hosted, ai, typescript]
```

#### RepositorySet vs multiple Repository files

You can achieve similar results with separate `Repository` files — one per repo. The trade-off:

| | RepositorySet | Multiple Repository files |
|---|---|---|
| Shared defaults | `defaults` block — write once | Repeated in each file |
| Adding a repo | Add 3 lines | Create a new file with full spec |
| Per-repo git blame | Harder (all in one file) | Clean (one file per repo) |
| YAML anchors | Work within the file | Cannot cross file boundaries (YAML spec limitation) |

Use RepositorySet when you have many repos with identical settings. Use separate files when each repo has distinct configuration or you want independent change tracking.

### FileSet

Distributes files to multiple repositories. Useful for keeping shared files (CODEOWNERS, LICENSE, etc.) in sync.

```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  name: common-files

spec:
  # Target repositories
  repositories:
    - babarot/gomi
    - babarot/enhancd
    - babarot/oksskolten
    - name: babarot/gh-infra
      overrides:
        - path: .github/CODEOWNERS
          content: |
            * @babarot @co-maintainer

  # Files to distribute
  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot

    - path: LICENSE
      source: ./templates/LICENSE    # Read from local file

    - path: .github/workflows
      source: ./templates/workflows/ # Sync entire directory

    - path: .github/SECURITY.md
      content: |
        ## Security Policy
        Please report vulnerabilities to security@example.com

  # What to do when a file has been manually edited (drift)
  on_drift: warn    # warn (default) | overwrite | skip

  # Custom commit message (optional)
  commit_message: "ci: sync shared config files"

  # Apply strategy (optional)
  strategy: direct           # direct (default) | pull_request
  # branch: gh-infra/sync   # branch name for pull_request strategy
```

#### File sources

Each file entry supports three modes:

- **`content`** — inline content in YAML
- **`source` (file)** — read content from a local file (`source: ./templates/LICENSE`)
- **`source` (directory)** — sync an entire directory (`source: ./templates/workflows/`). All files under the directory are expanded with paths relative to `path`.

Paths are resolved relative to the YAML file's location.

#### Apply strategy

By default, FileSet commits directly to the default branch using the [Git Data API](https://docs.github.com/en/rest/git/trees). All files are included in a single atomic commit regardless of file count.

Set `strategy: pull_request` to create a PR instead:

```yaml
spec:
  strategy: pull_request
  commit_message: "ci: sync shared files"
  # branch: gh-infra/custom-branch   # optional, auto-generated if omitted
```

This creates a branch, commits all files, and opens a PR. Reviewers can inspect the changes before merging.

For empty repositories (no commits yet), gh-infra automatically falls back to the Contents API, which creates one commit per file as the initial commit.

#### on_drift behavior

| Value | Plan | Apply |
|-------|------|-------|
| `warn` (default) | Shows drift warning | Skips the file |
| `overwrite` | Shows diff | Overwrites with desired content |
| `skip` | Ignores drift entirely | No action |

#### YAML anchors for DRY content

```yaml
_templates:
  codeowners: &codeowners |
    * @babarot

  license: &license |
    MIT License
    Copyright (c) 2025 babarot

spec:
  files:
    - path: .github/CODEOWNERS
      content: *codeowners
    - path: LICENSE
      content: *license
```

## Usage Patterns

### Central management

Manage all repos from a single dedicated repository:

```
github-config/
├── repos/
│   ├── gomi.yaml
│   ├── enhancd.yaml
│   ├── oksskolten.yaml
│   └── gh-infra.yaml
├── files/
│   └── common.yaml
└── gh-infra.yaml
```

```bash
gh infra plan ./repos/
gh infra apply ./repos/
```

### Self-managed

Each repo manages its own settings:

```
my-project/
├── .github/
│   ├── infra.yaml          # This repo's settings
│   └── workflows/
│       └── infra.yaml      # Auto-apply on merge
└── src/
```

Use `managed_by: self` in central mode to exclude self-managed repos.

### CI/CD integration

```yaml
# Auto-apply on merge
on:
  push:
    branches: [main]
    paths: [".github/infra.yaml"]
jobs:
  apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh infra apply .github/infra.yaml --auto-approve
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

```yaml
# Drift detection (scheduled)
on:
  schedule:
    - cron: "0 9 * * 1"   # Every Monday 9am
jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: gh infra plan ./repos/ --ci   # Exits 1 if drift detected
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Logging

Set the log level via `GH_INFRA_LOG` environment variable or `--log-level` flag:

```bash
GH_INFRA_LOG=debug gh infra plan ./repos/
gh infra plan ./repos/ --log-level=trace
```

| Level | What it shows |
|-------|---------------|
| `error` | Fetch failures |
| `warn` | gh command failures with stderr |
| `info` | Fetch targets, plan summary |
| `debug` | Every gh command executed, response sizes, diff results |
| `trace` | Everything above + full API response bodies (stdout/stderr) |

`--verbose` / `-V` is a shorthand for `--log-level=debug`.

Example with `trace` — useful for debugging API issues:

```
$ GH_INFRA_LOG=trace gh infra plan ./repos/

2026/03/21 03:03:04 INFO fetching repos=1 parallel=5
2026/03/21 03:03:04 DEBU exec cmd="gh repo view babarot/gh-infra --json ..."
2026/03/21 03:03:04 TRAC stdout cmd="gh repo view ..." output="{\"description\":\"...\", ...}"
2026/03/21 03:03:04 DEBU ok cmd="gh repo view ..." bytes=460
```

## Repository Lifecycle

### Creating repositories

If a YAML manifest references a repository that doesn't exist, `plan` shows it as a new resource:

```
$ gh infra plan ./repos/

Plan: 1 to create, 0 to update, 0 to destroy

  + babarot/new-project (new)
      + repository: babarot/new-project
```

`apply` creates it with `gh repo create`, then applies all settings (features, topics, branch protection, etc.) in a single pass.

### Archiving repositories

Set `archived: true` in the spec to mark a repository as read-only:

```yaml
spec:
  archived: true
```

This is a reversible operation — set it back to `false` to unarchive.

### Deleting repositories

gh-infra does **not** support repository deletion. Without a state file, there is no way to distinguish "removed from YAML" from "never managed" — and repository deletion is irreversible.

To delete a repository, use `gh repo delete` directly.

## License

MIT
