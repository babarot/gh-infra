---
title: Actions
sidebar:
  order: 8
---

Manage GitHub Actions permissions, SHA pinning, workflow defaults, allowed actions, and fork PR approval policies and workflow settings.

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
    fork_pr_approval: first_time_contributors   # public repositories
    fork_pr_workflows:                          # private and internal repositories
      run_workflows_from_fork_pull_requests: true
      send_write_tokens_to_workflows: false
      send_secrets_and_variables: false
      require_approval_for_fork_pr_workflows: true
```

## Permissions

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `enabled` | bool | | Enable or disable Actions for this repository. `false` stops all workflows. Required when any other actions field is set, due to a GitHub API limitation |
| `allowed_actions` | string | `all`, `local_only`, `selected` | Which Actions are allowed to run |
| `sha_pinning_required` | bool | | Require actions to be pinned to a full commit SHA instead of tags or branches |

:::note
While `enabled: false`, gh-infra neither applies nor compares `allowed_actions`, `sha_pinning_required` or `selected_actions`: GitHub rejects `allowed_actions` for a repository with Actions disabled and stops reporting it. Values set there, including ones inherited from a `RepositorySet`'s defaults, take effect again once `enabled` is `true`.
:::

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
| `patterns_allowed` | list | Glob patterns for allowed actions (e.g. `actions/*`, `owner/repo@*`). `[]` removes all patterns |

A field you leave out of `selected_actions` keeps its current value on GitHub, and `plan` compares only the fields you set.

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

`fork_pr_approval` is not available on private repositories. `validate` rejects it when the manifest sets `visibility: private`, and `plan` fails for the repository when GitHub reports the setting as unavailable.

## Fork PR Workflows

Control how workflows run for pull requests from forks of a **private or internal** repository. A public repository has [`fork_pr_approval`](#fork-pr-approval) instead; GitHub has one of the two for a repository, by its visibility.

```yaml
spec:
  visibility: private
  actions:
    enabled: true
    fork_pr_workflows:
      run_workflows_from_fork_pull_requests: true
      require_approval_for_fork_pr_workflows: true
```

| Field | Type | Description |
|-------|------|-------------|
| `run_workflows_from_fork_pull_requests` | bool | Run workflows for pull requests from forks |
| `send_write_tokens_to_workflows` | bool | Give those workflows a `GITHUB_TOKEN` with write permission |
| `send_secrets_and_variables` | bool | Give those workflows the repository's secrets and variables |
| `require_approval_for_fork_pr_workflows` | bool | Require approval before they run |

The fields are those of the GitHub API. Fields the manifest leaves out are left as they are; `apply` reads the current ones and sends all four, because GitHub requires `run_workflows_from_fork_pull_requests` in every request.

GitHub saves the last three as `false` while `run_workflows_from_fork_pull_requests` is `false`, and answers 204 to a request that sets them. Setting one of them to `true` there would show the same change after every `apply`, so it is an error: `validate` rejects it when the manifest sets both, and `plan` when it sets one and the repository has `run_workflows_from_fork_pull_requests: false` (or the manifest sets it to `false`).

`fork_pr_workflows` is not available on public repositories, as `fork_pr_approval` is not on private ones. `validate` rejects it when the manifest sets `visibility: public`, and `plan` fails for a repository that is public. In a `RepositorySet`, put it in the entries of the private and internal repositories, not in the defaults: a public repository inheriting it fails.

A repository the same plan makes private or internal is planned with every field the manifest sets, since GitHub has none while it is public; `apply` changes the visibility first.

## What Is Not Managed

The following Actions settings are **not yet supported** by gh-infra:

- Artifact and log retention period
- Cache retention and storage limits
- OIDC subject claim customization
- Custom deployment protection rules of environments (GitHub Apps); environments themselves are managed under [Environments](./environments/)
- Self-hosted runner configuration
