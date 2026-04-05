# ADR-002: Import action selector for interactive diff viewer

## Status

Proposed

## Context

The `import --into` command auto-determines a `WriteMode` for each file change, then presents an interactive diff viewer where users can only toggle `skip/apply`. This is insufficient for cases where the auto-determined mode is not what the user wants:

- A shared local source defaults to `WritePatch`, but the user may want to overwrite the template source (`write`) instead.
- A local source defaults to `WriteSource`, but the user may want to absorb the diff into a manifest patch instead.
- In all cases, the user may want to `skip` individual entries.

The existing `Skip bool` toggle on `DiffEntry` cannot express these choices.

## Decision

Introduce an `ImportAction` abstraction that decouples the user-facing choice from the internal `WriteMode`.

### User-facing actions

The diff viewer exposes exactly three actions:

| Action  | Meaning |
|---------|---------|
| `write` | Overwrite the local target (source file or inline content) |
| `patch` | Absorb the diff into the manifest patch section |
| `skip`  | Do nothing for this entry |

`source` vs `inline` distinction is hidden -- both map to `write` in the UI.

### Allowed actions per entry type

| Entry type | Allowed | Default | Rationale |
|------------|---------|---------|-----------|
| Template-based | `skip` | `skip` | Reverse transformation is not possible |
| `github://` source | `skip` | `skip` | Remote source, no local write target |
| Shared local source | `write`, `patch`, `skip` | `patch` | Avoid unintended cross-repo side effects |
| Inline content | `write`, `skip` | `write` | No patch concept for inline entries |
| Local source (single ref) | `write`, `patch`, `skip` | `write` | Source update is the normal case |

### Data model

`Change` gains three fields:

```go
type Change struct {
    // ...existing fields...
    SuggestedWriteMode WriteMode      // auto-determined default
    AllowedActions     []ImportAction  // valid choices for this entry
    SelectedAction     ImportAction    // user's final choice
}
```

### Resolution layer

A `ResolveWriteMode(Change) (WriteMode, error)` function converts the selected action back to a concrete `WriteMode` at write time:

- `ActionWrite` + inline-backed entry -> `WriteInline`
- `ActionWrite` + source-backed entry -> `WriteSource`
- `ActionPatch` -> `WritePatch`
- `ActionSkip` -> no-op

This keeps the existing `WriteInline`/`WriteSource`/`WritePatch` write paths unchanged.

### UI changes

`DiffEntry.Skip bool` is replaced by `Action ImportAction` and `AllowedActions []ImportAction`. The `Tab` key cycles through `AllowedActions` instead of toggling a boolean. The current action is displayed in the viewer pane.

### Diff semantics

The diff comparison target changes based on the selected action:

- **write**: local target (source file or manifest content) vs GitHub content
- **patch**: current patch result vs GitHub content
- **skip**: plain current view (no diff)

## Consequences

- Users can override the auto-determined write mode per entry without leaving the diff viewer.
- The mental model is slightly more complex (3 choices instead of 2), mitigated by showing only the allowed subset per entry and displaying the current action clearly.
- Existing default behavior is preserved when the user does not interact with the action selector.
- Template and `github://` entries remain read-only (`skip` only), avoiding unsafe operations.
- The `ResolveWriteMode` indirection adds one layer, but it isolates the UI model from the write implementation, making both independently testable.
- Future entry types only need to define their allowed actions and resolution rules.
