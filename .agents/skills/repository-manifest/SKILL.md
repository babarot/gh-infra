---
name: repository-manifest
description: >
  Complete YAML schema reference for Repository and RepositorySet resources.
  Use when writing or editing manifests for repo settings, branch protection,
  rulesets, secrets, or variables.
---

# Repository Manifest Reference

## Repository (kind: Repository)

Manages settings for a **single** repository.

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: babarot
  name: gomi
spec:
  # all fields below are optional — only declared fields are managed
```

### Basic Settings

```yaml
spec:
  description: "My awesome project"
  homepage: "https://example.com"
  visibility: public          # public | private | internal
  archived: false
  topics:
    - go
    - cli
```

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Repository description |
| `homepage` | string | URL displayed on the repo page |
| `visibility` | string | `public`, `private`, or `internal` (GitHub Enterprise) |
| `archived` | bool | Archive (read-only). Reversible — set `false` to unarchive |
| `topics` | list | GitHub topics for discoverability |

### Features

```yaml
spec:
  features:
    issues: true
    projects: false
    wiki: false
    discussions: false
```

All fields are `bool`. Omitted fields are not changed on GitHub.

### Merge Strategy

```yaml
spec:
  merge_strategy:
    allow_merge_commit: false
    allow_squash_merge: true
    allow_rebase_merge: false
    auto_delete_head_branches: true
    squash_merge_commit_title: PR_TITLE          # PR_TITLE | COMMIT_OR_PR_TITLE
    squash_merge_commit_message: COMMIT_MESSAGES  # COMMIT_MESSAGES | PR_BODY | BLANK
    merge_commit_title: MERGE_MESSAGE             # MERGE_MESSAGE | PR_TITLE
    merge_commit_message: PR_TITLE                # PR_TITLE | PR_BODY | BLANK
```

### Branch Protection (Classic)

Classic branch protection rules. For new setups, prefer rulesets (see below).

```yaml
spec:
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
```

| Field | Type | Description |
|-------|------|-------------|
| `pattern` | string | **Required.** Branch name or glob (e.g., `main`, `release/*`) |
| `required_reviews` | int | Number of required approving reviews |
| `dismiss_stale_reviews` | bool | Dismiss approvals on new commits |
| `require_code_owner_reviews` | bool | Require code owner review |
| `require_status_checks.strict` | bool | Require branch to be up to date |
| `require_status_checks.contexts` | list | Required status check names |
| `enforce_admins` | bool | Apply rules to admins too |
| `allow_force_pushes` | bool | Allow force pushes |
| `allow_deletions` | bool | Allow branch deletion |

Each `pattern` must be unique within the list.

### Rulesets (Modern)

The modern replacement for classic branch protection, with richer controls.

```yaml
spec:
  rulesets:
    - name: protect-main
      target: branch                # branch | tag (default: branch)
      enforcement: active           # active | evaluate | disabled
      bypass_actors:
        - role: admin               # role (admin|write|maintain), team, app, org-admin, or custom-role
          bypass_mode: always        # always | pull_request | exempt
      conditions:
        ref_name:
          include:
            - "refs/heads/main"
            - "refs/heads/release/*"
          exclude:
            - "refs/heads/sandbox/*"
      rules:
        pull_request:
          required_approving_review_count: 1
          dismiss_stale_reviews_on_push: true
          require_code_owner_review: false
          require_last_push_approval: false
          required_review_thread_resolution: false
        required_status_checks:
          strict_required_status_checks_policy: true
          contexts:
            - context: "ci/test"
              app: github-actions    # optional
        non_fast_forward: true       # toggle rules: true to enable
        deletion: true
        creation: false
        required_linear_history: false
        required_signatures: false
```

#### Bypass Actors

Exactly one actor type field per entry:

| Field | Description |
|-------|-------------|
| `role` | Built-in role: `admin`, `write`, or `maintain` |
| `team` | Organization team slug |
| `app` | GitHub App slug |
| `org-admin` | Set to `true` for organization administrators |
| `custom-role` | Enterprise Cloud custom role name |
| `bypass_mode` | `always` (pushes and PRs), `pull_request` (PRs only), or `exempt` (cannot bypass) |

#### Conditions

Use `fnmatch`-style patterns. Special values: `~DEFAULT_BRANCH`, `~ALL`.

#### Toggle Rules

Simple on/off rules — set to `true` to enable:

- `non_fast_forward` — block force pushes
- `deletion` — block ref deletion
- `creation` — block ref creation
- `required_linear_history` — require linear commit history
- `required_signatures` — require signed commits

Each `name` must be unique within the list.

### Secrets

```yaml
spec:
  secrets:
    - name: DEPLOY_TOKEN
      value: "${ENV_DEPLOY_TOKEN}"
    - name: SLACK_WEBHOOK
      value: "${ENV_SLACK_WEBHOOK}"
```

**Important:**

- Values **must** use `${ENV_*}` syntax to reference environment variables. Never put actual secret values in YAML.
- `plan` **cannot compare existing secret values** (GitHub API limitation). It only detects new secrets.
- Use `--force-secrets` flag with `apply` to re-set all secrets (e.g., after credential rotation).

Each `name` must be unique within the list.

### Variables

```yaml
spec:
  variables:
    - name: APP_ENV
      value: production
    - name: REGION
      value: us-central1
```

Variable values are plain text (not sensitive). `plan` can show full diffs for changes.

Each `name` must be unique within the list.

### Lifecycle

- **New repos**: If the repo doesn't exist on GitHub, `plan` shows it as "new" and `apply` creates it via `gh repo create`.
- **Archiving**: Set `archived: true` (reversible — set `false` to unarchive).
- **Deletion**: **Not supported.** Use `gh repo delete` directly.

---

## RepositorySet (kind: RepositorySet)

Manages settings for **multiple** repositories with shared defaults.

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: babarot          # no "name" field

defaults:
  spec:
    visibility: public
    features:
      wiki: false
      projects: false
    merge_strategy:
      allow_squash_merge: true
      auto_delete_head_branches: true

repositories:
  - name: gomi
    spec:
      description: "Trash CLI"
      topics: [go, cli]

  - name: enhancd
    spec:
      description: "Enhanced cd"
      topics: [zsh, shell]
      features:
        wiki: true        # overrides default
```

### Defaults and Override Merge Behavior

| Type | Behavior |
|------|----------|
| **Scalars** (string, bool, int) | Replaced by per-repo value |
| **Lists** (topics, branch_protection) | Replaced **entirely** (not appended) |
| **Maps** (features, merge_strategy) | Merged by key — only specified keys override |

If a per-repo entry does not specify a field, the default value is used.

### Complete Example

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: babarot

defaults:
  spec:
    visibility: public
    features:
      wiki: false
    merge_strategy:
      allow_squash_merge: true
      allow_rebase_merge: false
      auto_delete_head_branches: true
    rulesets:
      - name: protect-main
        target: branch
        enforcement: active
        conditions:
          ref_name:
            include: ["refs/heads/main"]
        rules:
          pull_request:
            required_approving_review_count: 1
          non_fast_forward: true

repositories:
  - name: gomi
    spec:
      description: "Trash CLI"
      topics: [go, cli]

  - name: enhancd
    spec:
      description: "Enhanced cd"
      topics: [zsh, shell]
      features:
        wiki: true
```

After writing a manifest, always run:

```bash
gh infra validate <path>
gh infra plan <path>
```
