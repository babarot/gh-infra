# Protection

## Reconcile

Collections (`rulesets`, `branch_protection`) are additive by default. Use top-level `reconcile` to opt into authoritative mode, which deletes remote entries not declared in YAML.

```yaml
reconcile:
  rulesets: authoritative
  branch_protection: additive

spec:
  rulesets:
    - name: protect-main
      ...
```

Modes:

- `additive` (default): create/update declared entries; undeclared remote entries are left untouched
- `authoritative`: YAML is the source of truth; undeclared remote entries are deleted

Rules:

- `reconcile` without the corresponding `spec` collection is a parse error (e.g., `reconcile.rulesets: authoritative` requires `spec.rulesets`)
- Use `spec.rulesets: []` with `reconcile.rulesets: authoritative` to delete all remote rulesets
- `rulesets: null` / `rulesets:` (explicit null) is invalid; use `[]` for empty managed collections
- `plan` output shows delete reason with the reconcile policy (e.g., `not declared; reconcile.rulesets=authoritative`)

## Rulesets

Prefer `rulesets` for new configurations.

```yaml
spec:
  rulesets:
    - name: protect-main
      target: branch
      enforcement: active
      bypass_actors:
        - role: admin
          bypass_mode: always
      conditions:
        ref_name:
          include: ["refs/heads/main"]
      rules:
        pull_request:
          required_approving_review_count: 1
        required_status_checks:
          strict_required_status_checks_policy: true
          contexts:
            - context: "ci/test"
              app: github-actions
        non_fast_forward: true
```

### Toggle Rules

Simple on/off rules — set to `true` to enable:

- `non_fast_forward` — block force pushes
- `deletion` — block ref deletion
- `creation` — block ref creation
- `required_linear_history` — require linear commit history
- `required_signatures` — require signed commits

### Conditions

Use `fnmatch`-style patterns. Special values: `~DEFAULT_BRANCH`, `~ALL`.

### Other Rules

- each ruleset `name` must be unique
- each bypass actor must set exactly one of `role`, `team`, `app`, `org-admin`, `custom-role`
- `bypass_mode`: `always`, `pull_request`, `exempt`

## Classic Branch Protection

Use when you need classic settings rather than rulesets:

```yaml
spec:
  branch_protection:
    - pattern: main
      required_reviews: 1
      dismiss_stale_reviews: true
      require_code_owner_reviews: false
      require_status_checks:
        strict: true
        contexts: ["ci / test"]
      enforce_admins: false
      allow_force_pushes: false
      allow_deletions: false
```

Each `pattern` must be unique.
