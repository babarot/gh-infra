# ADR-004: Deletable deletion markers without a YAML fork

## Status

Rejected

> This ADR records an implementation approach that was built and validated in
> [PR #138](https://github.com/babarot/gh-infra/pull/138), but intentionally
> closed without merge. The `Deletable[T]` implementation removed the YAML fork
> from ADR-003, but the broader schema question remained unresolved: whether
> collection deletion should be modeled as `field: null`, empty exact-set
> collections, or an explicit reconciliation policy.

## Context

ADR-003 explored and briefly implemented deletion markers using `Nullable[T]` plus a fork of `goccy/go-yaml`. That approach worked, but it had two design problems:

1. **The YAML fork put gh-infra-specific semantics into a general YAML decoder.** The fork changed null handling so `BytesUnmarshaler` was called for null nodes. This was carefully scoped, but it still meant maintaining a YAML decoder fork for a manifest-level feature.
2. **`Nullable[T]` named the mechanism, not the domain behavior.** In gh-infra, `branch_protection: null` does not merely mean "this field is nullable"; it means "delete existing remote resources of this type".

ADR-003's product-level requirement was carried forward during this iteration:

| YAML | Semantics |
|---|---|
| Field omitted | Do not manage this field |
| `field: null` / `field: ~` | Delete all existing remote resources of this type |
| `field:` with values | Manage these resources with the manifest values |

The implementation attempted to preserve gh-infra's stateless design, avoid a YAML library fork, and keep parser knowledge of individual repository fields low.

## Rejected Decision

Use a `Deletable[T]` manifest wrapper for fields where `null` means remote resource deletion.

```go
type Deletable[T any] struct {
    value    T
    isSet    bool
    isDelete bool
}
```

Initial use:

```go
type RepositorySpec struct {
    BranchProtection Deletable[[]BranchProtection] `yaml:"branch_protection,omitempty"`
    Rulesets         Deletable[[]Ruleset]          `yaml:"rulesets,omitempty"`
}
```

`Deletable[T]` is intentionally not a general nullable wrapper. It means:

- zero value: field omitted, so gh-infra does not manage that resource family
- delete marker: user wrote `field: null`, so gh-infra deletes existing remote resources
- populated value: user provided desired resources

The wrapped value is intentionally private. Callers must use constructors and
accessors so the three-state invariant cannot be bypassed by direct field
mutation:

```go
func NewDeletable[T any](v T) Deletable[T]
func DeleteValue[T any]() Deletable[T]
func (d Deletable[T]) IsSet() bool
func (d Deletable[T]) IsDelete() bool
func (d Deletable[T]) HasValue() bool
func (d Deletable[T]) Get() T
func (d Deletable[T]) GetOK() (T, bool)
func (d Deletable[T]) IsZero() bool
```

`IsNull()` is deliberately not used. Callers should ask whether a field is a delete marker, not whether it is syntactically null.

`Get()` returns the wrapped zero value when the field is unset or marked for
delete. This is useful for collection fields where a nil slice naturally means
"nothing to iterate". Callers that need to distinguish concrete values from
unset/delete states should use `GetOK()` or `HasValue()`.

Explicit YAML null is reserved for `Deletable[T]` fields. If a known
non-deletable field is written as `null`, parsing fails instead of silently
treating it as omitted:

```yaml
spec:
  labels: null # error: labels is not Deletable
```

Users should omit a field to leave it unmanaged. This prevents accidental
misreads such as assuming `labels: null` deletes all labels when it would
otherwise decode to the same state as an omitted field.

Slice fields also use helper functions to keep merge logic out of call sites:

```go
func HasItems[T any](d Deletable[[]T]) bool
func MergeDeletableSlice[T any](
    base Deletable[[]T],
    override Deletable[[]T],
    merge func([]T, []T) []T,
) Deletable[[]T]
```

### Parser-side null marker detection

Upstream `goccy/go-yaml` does not call `UnmarshalYAML` hooks when the YAML node is null. Instead of forking the decoder, gh-infra parses normally, then does a targeted raw YAML pass.

Parser responsibilities:

- parse `Repository` / `RepositorySet` using upstream `goccy/go-yaml`
- locate raw YAML maps for `RepositorySpec` instances:
  - `Repository.spec`
  - `RepositorySet.defaults.spec`
  - `RepositorySet.repositories[].spec`
- call a generic helper with the raw spec map and the decoded `RepositorySpec`

The helper uses reflection to find fields implementing an internal `deletableMarker` interface. It does not know about `branch_protection` or `rulesets`.

```go
func applyDeletableMarkers(raw map[string]any, dst any) error {
    // dst must be pointer to struct.
    // For each exported field:
    //   if field address implements deletableMarker:
    //     read its yaml tag key
    //     if raw[key] exists and is nil, mark the field for delete
    //   else if raw[key] exists and is nil:
    //     return a schema error
    //   else:
    //     recursively reject nested nulls
}
```

`Deletable[T]` exposes the internal marker method only inside the `manifest` package:

```go
type deletableMarker interface {
    markDelete()
}

func (d *Deletable[T]) markDelete() {
    var zero T
    d.value = zero
    d.isSet = true
    d.isDelete = true
}
```

This keeps the parser generic at the field level while still allowing null markers to be recovered from upstream go-yaml's null behavior.

### RepositorySet merge semantics

Deletion markers participate in `RepositorySet` merge with the same precedence as explicit values.

| defaults | entry override | merged result |
|---|---|---|
| unset | unset | unset |
| value | unset | value |
| delete | unset | delete |
| value | delete | delete |
| delete | value | value |
| value | value | merged value |

An entry-level value clears a defaults-level delete marker. An entry-level delete marker overrides a defaults-level value.

### Diff and apply semantics

Diff checks delete intent first:

```go
if desired.Spec.BranchProtection.IsDelete() {
    // Generate ChangeDelete for each current branch protection rule.
}

for _, bp := range desired.Spec.BranchProtection.Get() {
    // Diff concrete desired rules. If the field is omitted, Get() returns nil.
}
```

For branch protection, apply calls:

```text
DELETE /repos/{owner}/{repo}/branches/{pattern}/protection
```

For rulesets, diff stores the current ruleset ID in `Change.OldValue`, and apply calls:

```text
DELETE /repos/{owner}/{repo}/rulesets/{id}
```

Delete changes may carry `Children` for plan display, but `applyChange` must not recursively apply those children. The parent delete change is the only executable change.

## Alternatives Considered

### Ephemeral state

Rejected in ADR-003. A local or remote state file could detect that a field used to be managed and is now absent, but it would undermine gh-infra's stateless design and complicate CI/multi-user workflows.

### Empty collections as deletion markers

Rejected in ADR-003. Empty collections have legitimate value semantics and already interact with existing features such as `labels: []` plus `label_sync: mirror`. `null` is a clearer explicit marker.

### `Nullable[T]` plus a YAML fork

Rejected by ADR-003. It worked, but it required maintaining a `goccy/go-yaml` fork to call `BytesUnmarshaler` on null nodes. That moved gh-infra-specific manifest behavior into a general YAML decoder.

### `Nullable[T]` plus `nullable:"delete"` struct tags

Considered after removing the fork. This would keep `Nullable[T]` as a generic three-state wrapper and use a tag to specify that null means delete:

```go
BranchProtection Nullable[[]BranchProtection] `yaml:"branch_protection,omitempty" nullable:"delete"`
```

Rejected because current gh-infra use cases are specifically deletion semantics. A separate tag adds another invariant and creates tag-missing failure modes. Naming the type `Deletable[T]` makes the behavior explicit without extra tag metadata.

### Full YAML path engine

Rejected as too broad. A generic engine could collect null paths such as `$.repositories[2].spec.rulesets` and apply them to decoded structs. That requires general YAML-path-to-Go-value mapping, slice indexing, pointer handling, and nested struct traversal. gh-infra already knows where `RepositorySpec` nodes live, so a spec-level helper is simpler and better scoped.

## Expected Consequences If Adopted

### Positive

- No YAML fork is required; upstream `goccy/go-yaml` can be used directly.
- The type name matches the destructive domain behavior.
- Parser code no longer hard-codes `branch_protection` or `rulesets`.
- Future deletion-capable fields can use `Deletable[T]` without changing parser field-name lists.
- Tag-missing failures are avoided because the type itself carries the semantics.
- `field: null` remains explicit, stateless, and reviewable in plan output.
- Non-deletable fields cannot silently treat explicit null as omission.
- The wrapped value is private, so callers cannot accidentally construct
  inconsistent states such as "delete marker with a non-zero value".

### Negative / Tradeoffs

- The rename from `Nullable[T]` to `Deletable[T]` is a broad mechanical change across manifest, repository, importer, and tests.
- `Deletable[T]` must not be reused for non-delete null semantics. If gh-infra later needs scalar clearing behavior, it should introduce a separate type such as `Clearable[T]`.
- Parser code uses reflection to discover `Deletable[T]` fields. This is acceptable because it runs only during manifest parsing and over small schema structs.
- Parser code also rejects explicit nulls on non-deletable fields, including
  nested struct fields and slice elements. This makes `null` a reserved
  deletion-marker syntax rather than a generic "unset" spelling.
- `Deletable[T]` fields must have explicit YAML keys. The parser returns an internal schema error if a `Deletable[T]` field is tagged `yaml:"-"` or lacks a YAML key.

### Implementation Notes

- `MarshalYAML` returns `nil` for delete markers and the inner `value` for populated values.
- `IsZero()` returns true only for the omitted/unset state so `omitempty` drops unmanaged fields.
- `UnmarshalYAML` still handles non-null values and direct unit-test null calls, but normal manifest null recovery is parser-side because upstream go-yaml skips unmarshaler hooks for null nodes.
- `GetOK()` returns `ok=false` for both omitted and delete states. Use `IsSet()` and `IsDelete()` when those states must be distinguished.
- `MergeDeletableSlice` centralizes RepositorySet semantics: delete overrides all, non-empty override values merge, and omitted or empty overrides leave defaults unchanged.
- Export/import flows do not generate delete markers from GitHub state. They emit values or omit fields.

## Decision Outcome

Do not adopt `Deletable[T]` or `field: null` deletion markers at this time.

The implementation successfully demonstrated that null deletion markers can be
implemented without a YAML fork, but the approach was rejected before merge for
schema-design reasons:

- `field: null` only expresses full-collection deletion; it does not solve
  item-level deletion or exact-set reconciliation.
- YAML already distinguishes omitted keys, explicit null, empty sequences, and
  non-empty sequences. Those states may support a cleaner collection model than
  treating null as a destructive marker.
- A future design may introduce explicit collection reconciliation policy, such
  as a top-level `reconcile` block or collection object form, instead of
  separate `rulesets_sync` / `branch_protection_sync` fields.
- Introducing `field: null` now could create a breaking compatibility problem if
  the project later chooses a different canonical way to express collection
  deletion.

The collection deletion problem remains valid, but should be addressed by a
broader collection reconciliation ADR rather than this narrow `Deletable[T]`
mechanism.
