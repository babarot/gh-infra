# Actions

## Basic Shape

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
      patterns_allowed:
        - "actions/*"
    fork_pr_approval: all_external_contributors
```

## Validation Traps

- If any `actions.*` field other than `enabled` is set, `enabled` must also be set
- `selected_actions` requires `allowed_actions: selected`

## Important Fields

- `allowed_actions`: `all`, `local_only`, `selected`
- `sha_pinning_required`: require full SHA pinning for actions
- `workflow_permissions`: `read` or `write`
- `can_approve_pull_requests`: allow workflows to approve PRs
- `fork_pr_approval`: `first_time_contributors_new_to_github`, `first_time_contributors`, `all_external_contributors`

Prefer `workflow_permissions: read` and `can_approve_pull_requests: false` unless the repo needs stronger automation.
