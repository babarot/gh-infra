# Labels

## Basic Shape

```yaml
spec:
  labels:
    - name: kind/bug
      color: d73a4a
      description: "A bug; unintended behavior"
```

- `name` must be unique in the list
- `color` is hex without `#`

## Reconcile Mode

```yaml
reconcile:
  labels: authoritative

spec:
  labels:
    - name: kind/bug
      color: d73a4a
```

Modes:

- `additive`: create/update only; unmanaged labels remain
- `authoritative`: create/update/delete; unmanaged labels are removed

Use `authoritative` only when the manifest is authoritative. `plan` includes label usage info for pending deletions.

`spec.label_sync: mirror` is still accepted for compatibility, but it is deprecated.
