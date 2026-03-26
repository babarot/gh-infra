---
title: Actions
sidebar:
  order: 5
---

Manage GitHub Actions permissions, SHA pinning, workflow defaults, allowed actions, and fork PR approval policies.

## Example

```yaml
spec:
  actions:
    enabled: true
    allowed_actions: selected
    sha_pinning_required: true
    workflow_permissions: read
    can_approve_pull_requests: false
    selected_actions:
      github_owned_allowed: true
      verified_allowed: false
      patterns_allowed:
        - "actions/*"
        - "slackapi/slack-github-action@*"
    fork_pr_approval: first_time_contributors
```

## Permissions

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `enabled` | bool | | Enable or disable Actions for this repository. `false` stops all workflows. Required when any other actions field is set, due to a GitHub API limitation |
| `allowed_actions` | string | `all`, `local_only`, `selected` | Which Actions are allowed to run |
| `sha_pinning_required` | bool | | Require actions to be pinned to a full commit SHA instead of tags or branches |

:::tip
`sha_pinning_required: true` is a strong supply-chain hardening default for repositories that use third-party actions.
:::

### `allowed_actions` values

| Value | Description |
|-------|-------------|
| `all` | Any action from any repository can be used |
| `local_only` | Only actions defined in the same repository or owner can be used |
| `selected` | Only actions matching `selected_actions` patterns can be used |

## Workflow Permissions

Control the default permissions granted to `GITHUB_TOKEN` in workflows.

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `workflow_permissions` | string | `read`, `write` | Default `GITHUB_TOKEN` scope. GitHub recommends `read` (least privilege) |
| `can_approve_pull_requests` | bool | | Allow workflows to create and approve pull request reviews |

:::caution
`write` + `can_approve_pull_requests: true` allows workflows to approve and merge their own PRs. Use `read` + `false` unless you have a specific need.
:::

## Selected Actions

Only applies when `allowed_actions: selected`. Specifying `selected_actions` without `allowed_actions: selected` is a validation error.

```yaml
spec:
  actions:
    enabled: true
    allowed_actions: selected
    selected_actions:
      github_owned_allowed: true
      verified_allowed: false
      patterns_allowed:
        - "actions/*"
        - "slackapi/slack-github-action@*"
        - "my-org/*"
```

| Field | Type | Description |
|-------|------|-------------|
| `github_owned_allowed` | bool | Allow GitHub-owned actions (`actions/*`, `github/*`) |
| `verified_allowed` | bool | Allow actions by Marketplace verified creators |
| `patterns_allowed` | list | Glob patterns for allowed actions (e.g. `actions/*`, `owner/repo@*`) |

## Fork PR Approval

Control when manual approval is required before running workflows on pull requests from forks.

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `fork_pr_approval` | string | See below | Approval policy for fork PR workflows |

| Value | Description |
|-------|-------------|
| `first_time_contributors_new_to_github` | Require approval only for brand-new GitHub accounts (default) |
| `first_time_contributors` | Require approval for first-time contributors to this repository |
| `all_external_contributors` | Require approval for all contributors without write access (most strict) |

:::tip
For open-source projects, `all_external_contributors` is the safest choice to prevent malicious `pull_request_target` attacks.
:::

## What Is Not Managed

The following Actions settings are **not yet supported** by gh-infra:

- Artifact and log retention period
- Cache retention and storage limits
- OIDC subject claim customization
- Environments (deployment protection rules, reviewers, branch policies)
- Self-hosted runner configuration
- Fork PR settings for private repositories (`send_write_tokens`, `send_secrets`)
