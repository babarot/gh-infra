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
    labels: additive
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
- Simple lists: replaced entirely
- Keyed collections: merged by key
- Maps: merged by key
- Reconcile: merged by collection (per-repo overrides individual collections without resetting others)

Examples:

- `visibility`: scalar replace
- `reconcile.labels`, `reconcile.rulesets`, `reconcile.branch_protection`: merged by collection
- `topics`: list replace
- `labels`, `branch_protection`, `rulesets`, `secrets`, `variables`: merged by key
- `features`, `merge_strategy`, `actions`: map merge by key (individual fields like `enabled`, `allowed_actions` are independently overridable)
- `features.pull_requests`: map merge by key (`enabled` and `creation` are independently overridable)
- `actions.selected_actions`: map merge by key
- `actions.selected_actions.patterns_allowed`: list replace

If a repo entry omits a field, the default value remains active.

## Conditional Settings (`when` / `conditional_spec`)

```yaml
defaults:
  when:
    visibility: public        # public | private | internal
  conditional_spec:
    rulesets:
      - name: protect-main
repositories:
  - name: repo-a              # inherits when + conditional_spec
  - name: repo-b
    conditional_spec:         # merged on top of defaults.conditional_spec
      variables:
        - name: EXTRA
          value: "1"
```

- RepositorySet only (`defaults` and entries); not available on `kind: Repository`.
- `when` and `conditional_spec` must appear together at each level; `when` must set `visibility`.
- Entry `when` is optional; if present it must equal `defaults.when` (parse error otherwise). No per-entry opt-out.
- Evaluated at plan time against the repo's current GitHub visibility, not `spec.visibility`. New repos skip it; a visibility change needs a second apply before the conditional block applies.
- On match, `conditional_spec` is merged on top of `spec` with the normal merge rules, so it wins over `spec` for the same field/key.
- Validation traps: `conditional_spec.actions.fork_pr_approval` with `when.visibility: private` is rejected. `actions.enabled` may come from `spec.actions`; a partial `conditional_spec.actions` overlay is valid.
- `import --into` ignores `when` / `conditional_spec` when computing entry overrides.
