# ADR-007: Conditional `when:` clause for RepositorySetEntry

## Status

Proposed

## Context

GitHub Free and Team plans return HTTP 403 when applying rulesets to private
repositories. Currently, users who manage a mix of public and private repos in a
single RepositorySet must work around this by maintaining two separate YAML
files: one with rulesets (for public repos) and one without (for private repos).

This approach has several costs:

- Two files can diverge over time, causing drift between visibility classes.
- The relationship between files is implicit and only enforced by convention.
- `RepositorySet` cannot serve as a single source of truth for all repos in an
  organization if settings must be split across files.

The motivating concrete case is:

```yaml
repositories:
  - name: my-repo
    spec:
      description: "shared description"   # always applied
    when:
      visibility: public
    conditional_spec:
      rulesets:
        - name: protect-main
          ...
```

## Decision

Add a `when:` clause and `conditional_spec:` block to `RepositorySetEntry`.
When both are present, `conditional_spec` fields are applied only if the
condition is satisfied at plan time. When the condition is not met, those fields
are silently omitted from the changeset (no-op). Fields declared in `spec`
continue to apply unconditionally regardless of any `when:` clause.

### Evaluation model

Conditions are evaluated at plan time against `CurrentState.Visibility`, which
is already fetched by `FetchRepository` before `Diff` is called. Using current
state (not desired state) is intentional: the `when:` clause expresses a filter
on the repo's existing visibility, not on a visibility change declared in `spec`.

### Schema

```yaml
repositories:
  - name: my-repo
    spec:
      description: "always applied"
    when:
      visibility: public           # one of: public | private | internal
    conditional_spec:
      rulesets:
        - name: protect-main
          ...
```

Rules:

- `when:` and `conditional_spec:` must both be present or both absent. Either
  alone is a validation error.
- `when.visibility` must be one of `public`, `private`, or `internal`.
- `conditional_spec:` is NOT merged with `defaults.spec`. It is applied only to
  the entry it belongs to, and only when the condition is met.
- Scalar repo settings (`description`, `visibility`, `homepage`, etc.) can only
  be declared unconditionally in `spec`, not in `conditional_spec`. Conditional
  support for those fields can be added in a future iteration.

### Apply layer

`conditional_spec` items (rulesets, branch protection, labels, milestones,
secrets, variables) are looked up by name in `ConditionalSpec` when not found
in `Spec`, so the apply layer can dispatch them without additional changes to
the Change schema.

### New repo edge case

When a repository is being created (`current.IsNew`), `Diff` returns a single
`ChangeCreate` and the `when:` condition is never evaluated. On `createRepo`,
only `desired.Spec` is applied. This is acceptable: a new repo defaults to
private, so rulesets gated on `visibility: public` would correctly not apply
until the repo is made public.

### Import/export roundtrip

`OriginalCondition` and `OriginalConditional` are stored on `RepositoryDocument`
so the import layer can preserve the `when:` clause. However, `import --into`
currently modifies only `spec`, not `conditional_spec`. The `when:` clause is
preserved but conditional settings are not updated by import. This is a known
limitation for a future iteration.

## Alternatives Considered

### Negate at apply time via error recovery

Catch 403 from the GitHub API and silently skip the failed operation. Rejected
because silent skips on 403 mask real permission failures. A ruleset denied due
to plan misconfiguration and a ruleset denied due to GitHub plan restrictions
produce the same error; there is no way to distinguish them reliably.

### Add visibility filter to `defaults` block

Allow `defaults.when:` to filter the entire RepositorySet. Rejected because
this applies globally (all entries inherit the condition), not per-entry. The
goal is per-entry conditional settings within a mixed-visibility set.

### Per-field `skip_if_visibility` flags on rulesets

Add `skip_if_visibility_private: true` directly to Ruleset and BranchProtection
types. Rejected as too narrow: it only addresses one field type and encodes the
condition inside the leaf node rather than at the entry level, making it harder
to review and harder to extend to future condition types.

### Separate `when_spec` nesting inside `spec`

Embed the condition inline inside `spec` as `spec.when:`. Rejected because it
couples the condition to YAML parsing inside `RepositorySpec.UnmarshalYAML`,
which already has complex field-presence tracking logic. A sibling block at the
entry level (alongside `spec`, analogous to `reconcile:`) is consistent with
the existing schema structure.

## Consequences

### Positive

- Existing YAML without `when:` is unchanged; the feature is purely additive.
- Users with mixed-visibility RepositorySets can consolidate to a single file.
- The condition is visible at review time in the YAML diff, not hidden in apply
  logic.
- Extends the `reconcile:` sibling block pattern (ADR-005, ADR-006) rather than
  replacing it.

### Negative / Tradeoffs

- `conditional_spec` is NOT merged with defaults, by design. Users must
  repeat defaults in `conditional_spec` if they also need them there (unlikely
  given the scoping intent, but worth documenting).
- If a field appears in both `spec` and `conditional_spec` and the condition is
  met, the conditional value wins because `MergeSpecs` uses override semantics.
  Users should avoid declaring the same field in both blocks.
- New repo creation ignores `conditional_spec`. Users who want rulesets on a
  newly created public repo must apply twice: once to create the repo, then
  again after visibility is set.
- Import does not update `conditional_spec`. This is a known gap.
