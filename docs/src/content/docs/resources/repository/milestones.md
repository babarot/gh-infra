---
title: Milestones
sidebar:
  order: 3
---

Manage repository milestones declaratively. Milestones declared in the manifest are created or updated on GitHub. Existing milestones not listed in the manifest are left untouched.

```yaml
spec:
  milestones:
    - title: "v1.0"
      description: "First stable release"
      state: open
      due_on: "2026-06-01"
    - title: "v2.0"
      state: open
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | yes | Milestone title (must be unique within the list) |
| `description` | string | no | Description of the milestone's purpose |
| `state` | string | no | `open` (default) or `closed` |
| `due_on` | string | no | Due date in `YYYY-MM-DD` format (e.g., `2026-06-01`) |

## Additive Only

Milestones are always managed in additive mode — gh-infra creates and updates milestones but never deletes them. This is intentional: milestones typically have issues and pull requests attached, and removing a milestone would disassociate all linked items.

To retire a milestone, set its `state` to `closed`:

```yaml
spec:
  milestones:
    - title: "v1.0"
      state: closed
```

:::note
Unlike labels, there is no `reconcile.milestones: authoritative` mode. If you remove a milestone from the manifest, it remains on GitHub unchanged.
:::

## Defaults Inheritance

When using `RepositorySet`, per-repo milestones replace the default milestone list:

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: my-org
defaults:
  spec:
    milestones:
      - title: "v1.0"
        state: open
      - title: "v2.0"
        state: open
repositories:
  - name: inherits-milestones
    spec:
      description: "Gets v1.0 and v2.0 from defaults"
  - name: custom-milestones
    spec:
      milestones:
        - title: "alpha"
          state: open
```

In this example, `inherits-milestones` gets both default milestones, while `custom-milestones` gets only its own `alpha` milestone.

## Plan Output

Milestone changes are grouped under a `milestones` header in the plan output:

```
  ~ my-org/my-repo
      + milestones
          + v1.0              open due:2026-06-01 "First stable release"
          + v2.0              open
```

Updates show individual field changes:

```
  ~ my-org/my-repo
      ~ milestones
          ~ v1.0.state        open → closed
          ~ v1.0.due_on       2026-06-01 → 2026-07-01
```
