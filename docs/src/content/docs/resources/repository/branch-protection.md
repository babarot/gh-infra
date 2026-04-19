---
title: Branch Protection (Classic)
sidebar:
  order: 4
---

:::caution
This configures **classic** branch protection rules via the [Branch Protection API](https://docs.github.com/en/rest/branches/branch-protection). GitHub now recommends [Repository Rulesets](../rulesets/) as the successor, which offer richer controls such as enforcement modes (dry-run), granular bypass actors, tag protection, and audit history.

Classic branch protection still works and is fully supported by gh-infra, but for new setups consider using [`rulesets`](../rulesets/) instead.
:::

## Example

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

Multiple patterns can be defined. Each entry creates a separate branch protection rule.

## Deleting All Branch Protection Rules

Set `branch_protection` to `null` to delete all existing classic branch protection rules for the repository:

```yaml
spec:
  branch_protection: null
```

Omitting `branch_protection` is different: it leaves existing branch protection rules untouched and unmanaged by gh-infra.

:::caution
This deletes every classic branch protection rule on the repository. Run `gh infra plan` before `apply` to review the rules that will be removed.
:::

## Fields

| Field | Type | Description |
|-------|------|-------------|
| `pattern` | string | Branch name or pattern (e.g., `main`, `release/*`) |
| `required_reviews` | int | Number of required approving reviews |
| `dismiss_stale_reviews` | bool | Dismiss approvals when new commits are pushed |
| `require_code_owner_reviews` | bool | Require review from code owners |
| `require_status_checks.strict` | bool | Require branch to be up to date before merging |
| `require_status_checks.contexts` | list | Required status check names |
| `enforce_admins` | bool | Apply rules to admins too |
| `allow_force_pushes` | bool | Allow force pushes to matching branches |
| `allow_deletions` | bool | Allow deleting matching branches |

## Classic vs Rulesets

See [Rulesets vs Classic Branch Protection](../rulesets/#rulesets-vs-classic-branch-protection) for a detailed comparison and migration guidance.
