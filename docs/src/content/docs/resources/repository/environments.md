---
title: Environments
sidebar:
  order: 7
---

Deployment environments limit which branches and tags can deploy, and who must approve a deployment, before a job that names the environment gets its secrets or an OIDC token issued for it.

```yaml
reconcile:
  deployment_branch_policies: authoritative

spec:
  environments:
    - name: production
      wait_timer: 0
      prevent_self_review: true
      can_admins_bypass: false
      reviewers:
        - user: babarot
        - team: deployers
      deployment_branch_policy:
        custom_branch_policies:
          - name: main
          - name: "v*"
            type: tag
```

| Field | Type | Description |
|---|---|---|
| `name` | string | *Required.* The environment's name |
| `wait_timer` | int | Minutes to wait before a deployment starts, 0 to 43200. `0` removes the wait timer |
| `reviewers` | list | Users (`user:`) or teams (`team:`, the team's slug) who must approve a deployment, up to 6. `[]` removes all reviewers |
| `prevent_self_review` | bool | Whether the user who started a deployment is kept from approving it. Needs at least one reviewer |
| `can_admins_bypass` | bool | Whether administrators can bypass the protection rules. GitHub's default is `true` |
| `deployment_branch_policy` | object | Which branches and tags can deploy (see below) |

Fields the manifest leaves out are not compared and stay as they are on GitHub.

## Deployment branch policy

| YAML | Branches and tags that can deploy |
|---|---|
| left out | not managed; stays as it is on GitHub |
| `deployment_branch_policy: {}` | all branches and tags |
| `protected_branches: true` | protected branches only |
| `custom_branch_policies: [...]` | branches and tags that match a name pattern |
| `custom_branch_policies: []` | none |

A pattern has a `name` and a `type`, `branch` (the default) or `tag`. Patterns follow Ruby's `File.fnmatch`: `release/*` matches `release/1.0` but not `release/1.0/hotfix`.

`deployment_branch_policy:` with no value is invalid, since it reads too much like `{}`, which allows every branch. `protected_branches` and `custom_branch_policies` cannot be used together, and `protected_branches: false` alone means nothing (use `{}`).

When the policy stops being `custom_branch_policies`, GitHub deletes its patterns. `plan` shows each of them as `(removed by GitHub)`, whatever the reconcile mode.

## What gh-infra deletes

| What | Deleted when |
|---|---|
| An environment the manifest leaves out | Never. GitHub creates environments too (`github-pages`, or the first time a workflow names one) |
| A pattern the manifest leaves out | Only with `reconcile.deployment_branch_policies: authoritative`, in an environment whose `custom_branch_policies` the manifest sets |

`reconcile.deployment_branch_policies` has no `spec` collection of its name: it applies to the patterns of each declared environment.

## Names

GitHub matches environment names without regard to case, and does not rename an environment whose name differs only in case. gh-infra does the same: `Production` in the manifest is the `production` environment on GitHub, and the difference is not a change. Write names as GitHub has them.

## What `plan` checks

GitHub accepts some settings and then drops them, or rejects them only when applied. `plan` stops before that:

- **Reviewers who cannot read the repository.** GitHub drops such a reviewer without an error. `plan` checks each reviewer it adds: a user must be a collaborator (or the owner), a team must have access to the repository. Give them access first.
- **Plan limits on private repositories.** GitHub Free, Pro and Team allow wait timers and reviewers only on public repositories. When `apply` would send them to an environment of a private repository, `plan` reads the owner's plan and stops if it does not allow them. The plan of an organization can be read only with an organization owner's token, and a user's only with the user's own token with the `user` scope. If it cannot be read, `plan` accepts the settings only when an environment of the repository already has the same kind of rule, which shows the plan allows it.
- **`prevent_self_review` without reviewers.** GitHub keeps it with the reviewers, so it needs at least one.

| Repository | What GitHub allows |
|---|---|
| Public | Everything |
| Private or internal, GitHub Free | No environments |
| Private or internal, Pro or Team | Environments, deployment branch policies and `can_admins_bypass`; no wait timer and no reviewers |
| Private or internal, Enterprise | Everything |

## Changes that let more deploy

A change that widens who or what can deploy is easy to miss in a plan, so `plan` adds a note to the repository (a warning in `-o json` and `-o markdown`):

- the policy becomes `{}`: any branch or tag can deploy
- `reviewers` becomes `[]`: deployments no longer need a review
- `can_admins_bypass` becomes `true`

## New repositories

A new repository's environments are created by the same `apply`. GitHub drops a reviewer who cannot read the repository, and in a new one only its owner can, so `apply` adds the owner and leaves other reviewers (and `prevent_self_review`, when no reviewer is left) to a later `plan`, which adds them once they have access. `plan` says so in a note.

## What is not managed

- Custom deployment protection rules (GitHub Apps)
- Deleting an environment
