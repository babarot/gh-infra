# ADR-005: Collection reconciliation policy

## Status

Accepted

## Context

gh-infra is intentionally stateless. Every `plan` and `apply` fetches current
GitHub state and compares it with the YAML manifest. This avoids state files,
locking, and backend configuration, but it also means gh-infra cannot infer that
a resource removed from YAML should be deleted from GitHub.

ADR-003 and ADR-004 explored `field: null` as an explicit deletion marker for
collection fields such as `branch_protection` and `rulesets`. Both approaches
were rejected before merge:

- ADR-003 used `Nullable[T]` and a fork of `goccy/go-yaml`.
- ADR-004 replaced the fork with parser-side null detection and renamed the
  wrapper to `Deletable[T]`.

The implementations proved that explicit deletion markers are possible, but
the schema question remained unresolved. `field: null` can only express
full-collection deletion. It does not address the broader problem of choosing
whether a collection is managed additively or as an exact mirror of YAML.

YAML itself distinguishes the following states:

| YAML | YAML-level state |
|---|---|
| key omitted | absent |
| `rulesets:` | explicit null |
| `rulesets: null` | explicit null |
| `rulesets: []` | empty sequence |
| `rulesets: [ ... ]` | non-empty sequence |

This suggests a possible exact-set model:

| YAML | Possible meaning |
|---|---|
| key omitted | collection unmanaged |
| `rulesets: []` | managed empty collection; delete all remote rulesets |
| `rulesets: [ ... ]` | exact desired collection; delete remote rulesets not listed |
| `rulesets:` / `rulesets: null` | invalid |

That model is clean, but it would change existing behavior. Today, non-empty
collections are additive: gh-infra creates or updates listed entries and leaves
unlisted remote entries untouched. Changing list presence to mean exact-set
ownership would make existing manifests destructive.

We need a design that:

- preserves existing additive behavior by default
- allows users to opt into exact-set reconciliation
- keeps destructive behavior visible in the manifest
- avoids adding many `rulesets_sync`, `branch_protection_sync`, etc. fields to
  `spec`
- keeps GitHub desired state separate from gh-infra reconciliation behavior

## Decision

Introduce a top-level `reconcile` block for repository resources. `spec`
continues to describe desired GitHub resource values. `reconcile` describes how
gh-infra should compare those desired values with current remote state.

Initial form:

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: my-org
  name: my-repo

reconcile:
  rulesets: mirror
  branch_protection: additive

spec:
  rulesets:
    - name: protect-main
      target: branch
      rules:
        non_fast_forward: true
```

Initial supported collections:

| Reconcile field | Applies to |
|---|---|
| `rulesets` | `spec.rulesets` |
| `branch_protection` | `spec.branch_protection` |

Initial supported modes:

| Mode | Semantics |
|---|---|
| `additive` | Create/update entries declared in YAML. Leave undeclared remote entries untouched. |
| `mirror` | Make the remote collection match YAML exactly. Delete undeclared remote entries. |

Default mode is `additive`. This preserves existing behavior for manifests that
do not specify `reconcile`.

### Field presence semantics

`reconcile` only changes behavior for collections that are present in `spec`.

| YAML | Meaning |
|---|---|
| `spec.rulesets` omitted | rulesets unmanaged |
| `spec.rulesets` present, `reconcile.rulesets` omitted | additive management |
| `reconcile.rulesets: additive` + non-empty `spec.rulesets` | create/update listed rulesets only |
| `reconcile.rulesets: mirror` + non-empty `spec.rulesets` | exact-set management; delete remote rulesets not listed |
| `reconcile.rulesets: mirror` + `spec.rulesets: []` | managed empty collection; delete all remote rulesets |
| `rulesets:` / `rulesets: null` | invalid |

If `reconcile.rulesets` is set but `spec.rulesets` is omitted, parsing should
fail. A reconciliation policy without a corresponding desired collection is
ambiguous and likely a mistake.

The same rules apply to `branch_protection`.

### Empty sequences

Empty sequences are meaningful only when the collection is managed.

```yaml
reconcile:
  rulesets: mirror
spec:
  rulesets: []
```

This means "rulesets should be an empty collection" and deletes all repository
rulesets.

With `additive`, an empty list is valid but has no create/update work and does
not delete remote entries:

```yaml
reconcile:
  rulesets: additive
spec:
  rulesets: []
```

This is effectively a no-op for `rulesets`. The implementation may warn about
it, but it should not reinterpret additive empty lists as deletion.

### Null values

`null` is not used for collection deletion in this design.

```yaml
spec:
  rulesets:
```

and:

```yaml
spec:
  rulesets: null
```

should be parse errors. Users should write:

```yaml
reconcile:
  rulesets: mirror
spec:
  rulesets: []
```

to delete all remote rulesets.

### RepositorySet semantics

`RepositorySet` should support `reconcile` at both defaults and repository
entry levels:

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: my-org

defaults:
  reconcile:
    rulesets: mirror
  spec:
    rulesets:
      - name: protect-main
        rules:
          non_fast_forward: true

repositories:
  - name: repo-a
  - name: repo-b
    reconcile:
      rulesets: additive
```

Merge behavior:

- `defaults.reconcile` provides default reconciliation policy.
- `repositories[].reconcile` overrides defaults per collection.
- `defaults.spec` and `repositories[].spec` continue to merge as today.
- After merge, any reconcile policy that targets an omitted collection is an
  error.

This lets an organization use `mirror` by default while allowing individual
repositories to opt back into `additive`.

### Plan and apply behavior

Plan output should make mirror-driven deletes explicit. A delete caused by
mirror reconciliation is not the same as an update to a listed item; it is the
removal of a remote item that is absent from YAML.

Example wording:

```text
- ruleset "old-ruleset" (not declared; reconcile.rulesets=mirror)
```

Apply should execute those deletes only after they appear in plan, using the
existing GitHub delete APIs:

- branch protection: `DELETE /repos/{owner}/{repo}/branches/{pattern}/protection`
- rulesets: `DELETE /repos/{owner}/{repo}/rulesets/{id}`

### Import/export behavior

`import` and export flows should not emit `reconcile: mirror` by default.
Exporting current GitHub state into YAML should remain conservative and
non-destructive unless the user opts into mirror policy.

`import --into` should preserve an existing `reconcile` block. If a user already
declared `reconcile.rulesets: mirror`, import-into should update the listed
rulesets without silently removing the reconciliation policy.

## Alternatives Considered

### Keep `field: null` deletion markers

Rejected by ADR-004. `field: null` is concise for full-collection deletion, but
it does not solve item-level deletion or exact-set reconciliation. It also
reserves YAML null for destructive behavior, which may conflict with a future
canonical collection model.

### Infer exact-set ownership from list presence

Rejected for compatibility. This model is elegant:

```text
omitted = unmanaged
[]      = managed empty
[...]   = managed exact set
null    = invalid
```

But it changes existing non-empty list behavior from additive to exact-set,
which can delete remote resources that users expected gh-infra to leave alone.

### Add per-collection sibling fields in `spec`

Example:

```yaml
spec:
  rulesets_sync: mirror
  rulesets:
    - name: protect-main
```

Rejected as the primary direction. It follows the existing `label_sync` pattern,
but scales poorly as more collections need sync policy. It also mixes gh-infra
reconciliation behavior into `spec`, which otherwise describes desired GitHub
resource values.

### Add sync policy inside collection object form

Example:

```yaml
spec:
  rulesets:
    sync: mirror
    items:
      - name: protect-main
```

This keeps policy near items and avoids `rulesets_sync`, but it requires
list/object dual decoding for every collection. It also puts reconciliation
policy inside `spec`. This remains a possible future refinement, but top-level
`reconcile` provides a clearer separation of responsibilities.

### Use CLI flags or user config

Example:

```sh
gh infra apply ./infra --sync-mode=mirror
```

or:

```yaml
# ~/.config/gh-infra/config.yaml
repository:
  collection_sync: mirror
```

Rejected as the primary source of truth. Destructive behavior should be visible
in the manifest under review. User-global config can make the same manifest
produce different plans on different machines or in CI.

CLI flags may still be useful for preview or migration diagnostics, for example
to show what would be deleted if a collection were mirrored, but they should not
silently change normal apply semantics.

### Use YAML comments as directives

Example:

```yaml
spec:
  # gh-infra:sync=mirror
  rulesets:
    - name: protect-main
```

Rejected. Comments are not part of the YAML data model. Formatters, import,
export, and YAML edit tools may move or drop them. Destructive behavior should
be represented as data, not comments.

## Consequences

### Positive

- Existing manifests remain additive by default.
- Exact-set reconciliation becomes possible without state files.
- Full-collection deletion can be expressed without `null` by using
  `reconcile.<collection>: mirror` plus an empty list.
- Destructive behavior is visible in the manifest and can be reviewed in PRs.
- `spec` remains focused on desired GitHub resource values.
- `reconcile` creates a single place for future reconciliation policy instead
  of adding many `xxx_sync` fields.

### Negative / Tradeoffs

- The schema becomes larger: `Repository`, `RepositorySet.defaults`, and
  `RepositorySet.repositories[]` need `reconcile` blocks.
- Users must learn a new top-level concept.
- `reconcile` and `spec` can become inconsistent, so validation must catch
  policies targeting omitted collections.
- Mirror mode can delete remote resources created manually or by other tools.
  Plan output must make that reason explicit.
- `label_sync` remains a pre-existing special case. A later ADR may decide
  whether to keep it, deprecate it, or migrate labels into the same `reconcile`
  model.

### Implementation Notes

- Start with `rulesets` and `branch_protection` only.
- Keep `additive` as the default for both collections.
- Reject explicit null for these collection fields.
- Track field presence during parsing so `omitted`, `[]`, and non-empty lists
  remain distinguishable.
- In mirror mode, diff should generate `ChangeDelete` for current remote entries
  not present in the desired collection.
- In additive mode, diff should not generate deletes for undeclared remote
  entries.
- `plan` should include the reconcile policy in delete reasons.
- `apply` can reuse existing delete APIs once diff emits delete changes.
