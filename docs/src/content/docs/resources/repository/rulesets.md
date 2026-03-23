---
title: Rulesets
sidebar:
  order: 3
---

Configure [GitHub repository rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets) — the modern replacement for classic branch protection rules, with richer controls and enforcement modes.

:::note
Rulesets and `branch_protection` are independent features. You can use either or both. Classic branch protection uses the [Branch Protection API](../branch-protection/), while rulesets use the [Repository Rulesets API](https://docs.github.com/en/rest/repos/rules).
:::

## Example

```yaml
spec:
  rulesets:
    - name: protect-main
      target: branch
      enforcement: active
      bypass_actors:
        - actor_id: 5
          actor_type: RepositoryRole
          bypass_mode: always
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
              integration_id: 123
        non_fast_forward: true
        deletion: true
        creation: false
        required_linear_history: false
        required_signatures: false

    - name: protect-tags
      target: tag
      enforcement: evaluate
      conditions:
        ref_name:
          include: ["refs/tags/v*"]
      rules:
        deletion: true
        non_fast_forward: true
```

## Top-level Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | *required* | Ruleset name (must be unique within the repository) |
| `target` | string | `branch` | What the ruleset applies to: `branch` or `tag` |
| `enforcement` | string | `active` | `active` (enforced), `evaluate` (dry-run, logged but not enforced), or `disabled` |
| `bypass_actors` | list | `[]` | Actors who can bypass this ruleset |
| `conditions` | object | — | Which branches or tags the ruleset applies to |
| `rules` | object | *required* | The rules to enforce |

## Bypass Actors

Each entry allows a specific actor to bypass the ruleset:

```yaml
bypass_actors:
  - actor_id: 5
    actor_type: RepositoryRole
    bypass_mode: always
```

| Field | Type | Description |
|-------|------|-------------|
| `actor_id` | int | Numeric ID of the actor. For `RepositoryRole`: `5` = Admin, `4` = Write |
| `actor_type` | string | `RepositoryRole`, `Team`, `Integration`, or `OrganizationAdmin` |
| `bypass_mode` | string | `always` (pushes and PRs) or `pull_request` (PRs only) |

## Conditions

Specify which branches or tags the ruleset targets using `fnmatch`-style patterns:

```yaml
conditions:
  ref_name:
    include:
      - "refs/heads/main"
      - "refs/heads/release/*"
    exclude:
      - "refs/heads/sandbox/*"
```

Special values: `~DEFAULT_BRANCH` matches the repository's default branch, `~ALL` matches all branches/tags.

## Rules

### `pull_request`

Require pull request reviews before merging:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `required_approving_review_count` | int | `0` | Number of required approving reviews (0–10) |
| `dismiss_stale_reviews_on_push` | bool | `false` | Dismiss approvals when new commits are pushed |
| `require_code_owner_review` | bool | `false` | Require review from code owners |
| `require_last_push_approval` | bool | `false` | Last pusher cannot self-approve |
| `required_review_thread_resolution` | bool | `false` | All review threads must be resolved |

### `required_status_checks`

Require specific CI checks to pass:

```yaml
rules:
  required_status_checks:
    strict_required_status_checks_policy: true
    contexts:
      - context: "ci/test"
        integration_id: 123
      - context: "ci/lint"
```

| Field | Type | Description |
|-------|------|-------------|
| `strict_required_status_checks_policy` | bool | Require branch to be up to date before merging |
| `contexts[].context` | string | Name of the required status check |
| `contexts[].integration_id` | int | *(optional)* GitHub App ID that must provide this check |

### Toggle Rules

Simple on/off rules — set to `true` to enable:

| Field | Description |
|-------|-------------|
| `non_fast_forward` | Block force pushes to matching refs |
| `deletion` | Block deletion of matching refs |
| `creation` | Block creation of matching refs |
| `required_linear_history` | Require linear commit history (no merge commits) |
| `required_signatures` | Require signed commits |

## Rulesets vs Classic Branch Protection

| Feature | Classic `branch_protection` | `rulesets` |
|---------|----------------------------|------------|
| Multiple rules per branch | No (one rule per pattern) | Yes (rules stack) |
| Enforcement modes | On/off only | Active, evaluate (dry-run), disabled |
| Bypass control | Admin override only | Granular per role/team/app |
| Tag protection | No | Yes (`target: tag`) |
| Audit trail | No | Built-in version history |
| Merge queue | Separate config | Integrated as a rule type (Phase 2) |
| Metadata rules | No | Commit message patterns, etc. (Phase 2) |
