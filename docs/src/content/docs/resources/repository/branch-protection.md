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
| `require_status_checks.strict` | bool | Require branch to be up to date before merging. `false` when `require_status_checks` is written without it |
| `require_status_checks.contexts` | list | Required status check names. Leaving it out keeps the current checks |
| `enforce_admins` | bool | Apply rules to admins too |
| `restrict_pushes` | bool | Restrict who can push to matching branches. `true` adds a restriction (only admins can push, unless users or teams are already allowed on GitHub); `false` removes it. Organization repositories only |
| `allow_force_pushes` | bool | Allow force pushes to matching branches |
| `allow_deletions` | bool | Allow deleting matching branches |

## Updating an existing rule

GitHub replaces the whole protection of a branch on update, so gh-infra reads the current protection and changes only what the manifest sets. Everything the manifest leaves out keeps its current value, including the settings gh-infra does not model: `require_last_push_approval`, dismissal restrictions, users and teams that can bypass pull requests or push, `required_linear_history`, `required_conversation_resolution`, `block_creations`, `lock_branch`, and `allow_fork_syncing`. `plan` compares only what the manifest sets, which is what `apply` changes.

A few things to know:

- Writing `require_status_checks.contexts` replaces the required checks. The app each check is tied to on GitHub is not kept, since the manifest has no field for it; leave `contexts` out to keep the checks as they are.
- A change made on GitHub to a setting the manifest leaves out stays and does not show up in `plan`. Set what you want enforced explicitly.
- If the current protection cannot be read, the update fails instead of being applied from the manifest alone.

## Classic vs Rulesets

See [Rulesets vs Classic Branch Protection](../rulesets/#rulesets-vs-classic-branch-protection) for a detailed comparison and migration guidance.
