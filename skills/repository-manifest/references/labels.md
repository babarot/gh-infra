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

## Sync Mode

```yaml
spec:
  label_sync: mirror
  labels:
    - name: kind/bug
      color: d73a4a
```

Modes:

- `additive`: create/update only; unmanaged labels remain
- `mirror`: create/update/delete; unmanaged labels are removed

Use `mirror` only when the manifest is authoritative. `plan` includes label usage info for pending deletions.
