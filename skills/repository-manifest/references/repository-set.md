# RepositorySet

## Shape

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: my-org
defaults:
  reconcile:
    rulesets: authoritative
  spec:
    visibility: public
    features:
      wiki: false
    rulesets:
      - name: protect-main
repositories:
  - name: repo-a
    spec:
      description: "Repo A"
  - name: repo-b
    reconcile:
      rulesets: additive
```

## Merge Rules

- Scalars: replaced
- Lists: replaced entirely
- Maps: merged by key
- Reconcile: merged by collection (per-repo overrides individual collections without resetting others)

Examples:

- `visibility`, `label_sync`: scalar replace
- `topics`, `labels`, `branch_protection`, `rulesets`, `secrets`, `variables`: list replace
- `features`, `merge_strategy`, `actions`: map merge by key (individual fields like `enabled`, `allowed_actions` are independently overridable)
- `features.pull_requests`: map merge by key (`enabled` and `creation` are independently overridable)
- `actions.selected_actions`: map merge by key
- `actions.selected_actions.patterns_allowed`: list replace

If a repo entry omits a field, the default value remains active.
