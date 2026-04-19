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
reconcile:
  branch_protection: additive

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

## Reconcile Mode

Classic branch protection is additive by default. gh-infra creates and updates rules listed in `spec.branch_protection`, but it does not delete branch protection rules that exist on GitHub and are missing from YAML.

Set `reconcile.branch_protection: authoritative` to make GitHub branch protection rules match YAML exactly:

```yaml
reconcile:
  branch_protection: authoritative
spec:
  branch_protection:
    - pattern: main
      required_reviews: 1
```

With `authoritative`, any classic branch protection rule not declared in `spec.branch_protection` is planned for deletion. To delete all classic branch protection rules:

```yaml
reconcile:
  branch_protection: authoritative
spec:
  branch_protection: []
```

`branch_protection: null` and `branch_protection:` are invalid. Use `[]` for an explicitly empty managed collection.

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
