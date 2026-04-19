# ADR-003: Nullable deletion markers for stateless resource removal

## Status

Accepted

## Context

### The problem: stateless tools cannot express "delete"

gh-infra is deliberately stateless -- every `plan` and `apply` fetches live GitHub state and diffs it against the YAML manifest. There is no `.tfstate` equivalent. This eliminates an entire class of problems (state corruption, locking, backend configuration), but it introduces a semantic gap:

**When a user removes a field from their YAML, gh-infra cannot distinguish between "I never managed this" and "I used to manage this and now I want it gone".**

Concretely, consider a user who had `branch_protection` in their manifest, applied it, and then deleted the YAML block:

```yaml
# Before                          # After
spec:                              spec:
  branch_protection:                 # (field removed)
    - pattern: main
      required_reviews: 1
```

After the deletion, `plan` produces no changes -- the field is absent from YAML, so gh-infra treats it as "not managed". The branch protection rule remains on GitHub indefinitely with no way to remove it through the tool.

### Rejected alternative: ephemeral state ("disposable state")

One approach is to write a local state file after each `apply`, recording which fields were managed. On the next `plan`, the tool can compare the previous state against the current manifest and detect removals.

This was considered and rejected because:

1. **It reintroduces the state management burden** that the stateless design deliberately avoids. Even if the state file is "disposable" (can be deleted without catastrophic consequences), users must still understand when it exists, where it lives, and what happens when it goes missing.
2. **Multi-user and CI/CD workflows** would need state sharing or regeneration, re-creating the coordination problems of remote state backends.
3. **The problem scope is narrow** -- only a handful of collection-typed fields (branch protection, rulesets, and potentially secrets/variables) need deletion semantics. A per-field solution is simpler than a whole-system state layer.

## Decision

### Use `null` as an explicit deletion marker

YAML's native `null` value (or its alias `~`) is repurposed as a three-state signal:

| YAML | Semantics |
|---|---|
| Field omitted | "Don't manage this field" (existing behavior, unchanged) |
| `field: null` | "Delete all existing resources of this type" |
| `field:` with values | "Create or update these resources" (existing behavior, unchanged) |

Example:

```yaml
spec:
  branch_protection: null   # Delete all branch protection rules
  rulesets: null             # Delete all rulesets
```

### Why `null` over empty collections

An empty collection (`branch_protection: []`) was considered but rejected because it creates ambiguity with legitimate "zero items" semantics. For example, `labels: []` combined with `label_sync: mirror` already means "delete all labels" in the existing codebase. Using `null` provides a distinct, unambiguous signal that is orthogonal to empty-vs-populated.

### Implementation: `Nullable[T]` generic type

A `Nullable[T]` wrapper type (in `internal/manifest/nullable.go`) encodes the three states:

```go
type Nullable[T any] struct {
    Value  T
    isSet  bool
    isNull bool
}
```

- **Zero value** (`isSet=false`) → field was omitted from YAML
- **Null** (`isSet=true, isNull=true`) → user wrote `field: null`
- **Populated** (`isSet=true, isNull=false`) → user provided values in `Value`

The type implements `BytesUnmarshaler` to detect null during YAML parsing:

```go
func (n *Nullable[T]) UnmarshalYAML(data []byte) error {
    n.isSet = true
    if isYAMLNull(data) {
        n.isNull = true
        return nil
    }
    return yaml.Unmarshal(data, &n.Value)
}
```

It also implements `MarshalYAML` (to serialize correctly) and `IsZero` (to support `omitempty`).

### Prerequisite: forked goccy/go-yaml

The upstream [goccy/go-yaml](https://github.com/goccy/go-yaml) library (v1.19.2) does not call any `UnmarshalYAML` interface when the YAML value is null -- it silently sets the field to its zero value. This was verified experimentally across all three unmarshaler interfaces (`BytesUnmarshaler`, `InterfaceUnmarshaler`, `NodeUnmarshaler`).

The root cause is in `decode.go`, function `createDecodedNewValue`:

```go
// Upstream: null skips decodeValue entirely
if node.Type() != ast.NullType {
    if err := d.decodeValue(ctx, newValue, node); err != nil {
        return reflect.Value{}, err
    }
}
```

A [fork](https://github.com/babarot/go-yaml) adds an `else if` branch that calls `BytesUnmarshaler` on null nodes:

```go
} else if newValue.CanAddr() {
    if u, ok := newValue.Addr().Interface().(BytesUnmarshaler); ok {
        b, _ := d.unmarshalableDocument(node)  // produces []byte("null")
        u.UnmarshalYAML(b)
    }
}
```

**Only `BytesUnmarshaler` is affected.** `InterfaceUnmarshaler` and `NodeUnmarshaler` retain the existing skip-on-null behavior. This is critical because the existing `PullRequests` type implements `InterfaceUnmarshaler` (supporting both bool and object YAML forms), and calling it on null would misinterpret `pull_requests: null` as `pull_requests: false`.

The fork also includes [PR #864](https://github.com/goccy/go-yaml/pull/864) (LiteralNode Replace indentation fix), which is also pending upstream.

gh-infra references the fork via a `replace` directive in `go.mod`:

```
replace github.com/goccy/go-yaml => github.com/babarot/go-yaml v1.19.2-babarot.1
```

If upstream merges equivalent changes, the `replace` can be removed.

### Scope: `branch_protection` and `rulesets` only

The initial implementation applies `Nullable[T]` to two fields:

- `RepositorySpec.BranchProtection` → `Nullable[[]BranchProtection]`
- `RepositorySpec.Rulesets` → `Nullable[[]Ruleset]`

These were chosen because:

1. Both have well-defined GitHub DELETE APIs (`DELETE /branches/{pattern}/protection`, `DELETE /rulesets/{id}`)
2. Both are commonly configured then later removed
3. Labels already have deletion semantics via `label_sync: mirror`

Future fields (secrets, variables) can adopt `Nullable[T]` by changing the field type -- no other infrastructure changes are needed.

### How deletion flows through the system

1. **Parse**: YAML `branch_protection: null` triggers `Nullable[T].UnmarshalYAML` which sets `isNull=true`
2. **Validate**: Null fields skip element-level validation (no elements to validate)
3. **Merge** (RepositorySet): A null override propagates to the merged result. A populated override clears any null from defaults. `Nullable[T]` is a value type, so `result := defaults.Spec` produces an independent copy without shared-reference bugs.
4. **Diff**: `diffBranchProtection` / `diffRulesets` check `IsNull()` first. If true, generate `ChangeDelete` for every entry in `CurrentState`, with children showing current values for display.
5. **Plan display**: Delete changes render with `-` icons, and children show what will be removed:
   ```
     - branch protection "main"
         - required_reviews              1
         - enforce_admins                true
         - allow_force_pushes            false
   ```
6. **Apply**: `applyBranchProtection` / `applyRuleset` handle `ChangeDelete` by calling the appropriate DELETE API. For rulesets, the ID is carried in `Change.OldValue` as a `rulesetDeleteInfo` struct (resolved from `CurrentState` at diff time, avoiding an extra API call at apply time).
7. **Export** (`import` command): `ToManifest()` always produces populated values from current state -- it never generates null markers.
8. **Import --into**: If the local manifest has a null field, import skips it (the user's explicit deletion intent is preserved).

### Changes to `applyChange` child expansion

The existing `applyChange` function expands `Children` recursively for most change types. Delete changes carry children purely for display purposes (showing what will be removed), not for execution. A guard was added:

```go
if len(c.Children) > 0 && c.Type != ChangeDelete && ...
```

This ensures delete children are not dispatched as individual API calls.

## Consequences

### What becomes easier

- **Deleting managed resources**: Users can now write `branch_protection: null` and run `apply` to remove all branch protection rules. Previously, this required manual API calls or the GitHub UI.
- **Extending to more fields**: Adding null-deletion support to a new field requires only changing its type to `Nullable[T]` and adding the corresponding delete handler in `diff.go` and `apply.go`.
- **No state management**: The stateless property of gh-infra is fully preserved. There are no state files to manage, share, or recover.

### What becomes more difficult or requires attention

- **Fork maintenance**: The go-yaml fork must track upstream releases. In practice, goccy/go-yaml receives ~1-2 bugfix commits per month, so the maintenance burden is low. An upstream PR for the null BytesUnmarshaler change would eliminate the fork.
- **`.Value` access**: All code that accesses `BranchProtection` or `Rulesets` must now use `.Value` to get the underlying slice. This affects ~30 call sites across diff, apply, export, importer, and tests. The compiler catches all missing `.Value` accesses at build time (type mismatch errors), so there is no risk of silent breakage.
- **Validation tag removal**: The `validate:"unique=pattern"` and `validate:"unique=name"` struct tags were removed because the tag validator uses reflection and cannot inspect inside the `Nullable` wrapper. Uniqueness checks are now performed manually in `validation.go`. This is a minor loss of co-location but the checks are straightforward.
- **User education**: Users need to learn that `field: null` means "delete" and field omission means "don't manage". This is a new concept that must be documented. The distinction is intuitive for users familiar with Terraform's `null` behavior or YAML's native null semantics.
