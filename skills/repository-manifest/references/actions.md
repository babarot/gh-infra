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
    fork_pr_approval: all_external_contributors   # public repos only
    fork_pr_workflows:                            # private / internal repos only
      run_workflows_from_fork_pull_requests: true
      send_write_tokens_to_workflows: false
      send_secrets_and_variables: false
      require_approval_for_fork_pr_workflows: true
```

- `selected_actions` fields left out keep their current values; `patterns_allowed: []` removes all patterns
- With `enabled: false`, `allowed_actions`, `sha_pinning_required` and `selected_actions` are neither sent nor compared (GitHub rejects `allowed_actions` while Actions is disabled)

## Validation Traps

- If any `actions.*` field other than `enabled` is set, `enabled` must also be set
- `selected_actions` requires `allowed_actions: selected`
- `fork_pr_workflows` is for private / internal repos only: `validate` rejects it with `visibility: public`; `plan` fails a repo that is public. `fork_pr_approval` is the reverse. In a RepositorySet put it in the private entries, not in defaults (a public entry inheriting it fails)
- `fork_pr_workflows`: GitHub saves `send_write_tokens_to_workflows`, `send_secrets_and_variables` and `require_approval_for_fork_pr_workflows` as `false` while `run_workflows_from_fork_pull_requests` is `false` (answering 204), so `true` for them there never converges: `validate` rejects it when the manifest sets both, `plan` when the effective run (manifest, else GitHub) is `false`, or, for a new repo or one made private in the same plan, when the manifest does not set run to `true`. Turning run off plans the other fields that are on as `false` too

## Important Fields

- `allowed_actions`: `all`, `local_only`, `selected`
- `sha_pinning_required`: require full SHA pinning for actions
- `workflow_permissions`: `read` or `write`
- `can_approve_pull_requests`: allow workflows to approve PRs
- `fork_pr_approval`: `first_time_contributors_new_to_github`, `first_time_contributors`, `all_external_contributors`; not available on private repositories (validate rejects it with `visibility: private`; otherwise plan fails the repo on GitHub's 422)
- `fork_pr_workflows.*`: four bools, API names; left-out fields keep their value; `apply` reads the current four and PUTs all of them (`run_workflows_from_fork_pull_requests` is required in every PUT), then reads again and fails if GitHub did not keep them. A repo the same plan makes private is planned with every field the manifest sets

Prefer `workflow_permissions: read` and `can_approve_pull_requests: false` unless the repo needs stronger automation.
