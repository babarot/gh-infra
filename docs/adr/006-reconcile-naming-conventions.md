# ADR-006: Reconcile naming conventions

## Status

Accepted

## Context

gh-infra has reconciliation policy fields in multiple resource types.

Repository resources use a top-level `reconcile` block to control how managed
collections are compared with current GitHub state:

```yaml
apiVersion: gh-infra/v1
kind: Repository
metadata:
  owner: my-org
  name: my-repo

reconcile:
  labels: authoritative
  rulesets: authoritative
  branch_protection: additive

spec:
  labels:
    - name: kind/bug
      color: d73a4a
  rulesets:
    - name: protect-main
      target: branch
      rules:
        non_fast_forward: true
```

File and FileSet resources use `reconcile` on each file entry:

```yaml
apiVersion: gh-infra/v1
kind: File
metadata:
  owner: my-org
spec:
  files:
    - path: config/app.yaml
      source: templates/app.yaml
      reconcile: mirror
```

The same conceptual policies currently use different names:

| Concept | Repository | File/FileSet |
|---|---|---|
| Create/update declared entries and leave undeclared remote entries untouched | `additive` | `patch` |
| Treat YAML as the source of truth and delete undeclared remote entries | `authoritative` | `mirror` |
| Create once and never update after creation | N/A | `create_only` |

This forces users to learn different terms for the same reconciliation behavior
inside one tool. The mismatch will become more costly as gh-infra gains more
resources and managed collections.

## Decision

Standardize reconciliation policy names across resource types:

| Value | Meaning | Applies to |
|---|---|---|
| `additive` | Create/update declared entries. Leave undeclared remote entries untouched. | Repository, File/FileSet |
| `authoritative` | Treat YAML as authoritative for the managed scope. Delete undeclared remote entries in that scope. | Repository, File/FileSet |
| `create_only` | Create if missing. Do not update after creation. | File/FileSet |

Repository already uses `additive` and `authoritative`. File and FileSet should
migrate from `patch` and `mirror` to `additive` and `authoritative`.

### Repository Semantics

Repository `reconcile.<collection>` is a policy for a managed collection. It is
not an ownership declaration by itself.

A collection becomes managed only when the merged `spec.<collection>` is present
in YAML. If a collection is omitted from `spec`, it remains unmanaged even when
`reconcile` contains a policy for that collection.

| YAML | YAML-level state | gh-infra meaning |
|---|---|---|
| `spec.rulesets` omitted | absent | unmanaged; no-op even if `reconcile.rulesets` is set |
| `spec.rulesets:` | explicit null | invalid |
| `spec.rulesets: null` | explicit null | invalid |
| `spec.rulesets: []` | empty sequence | managed empty collection |
| `spec.rulesets: [ ... ]` | non-empty sequence | managed collection |

With `authoritative`:

| YAML | Meaning |
|---|---|
| `reconcile.rulesets: authoritative` + `spec.rulesets` omitted | rulesets unmanaged; no-op |
| `reconcile.rulesets: authoritative` + `spec.rulesets: []` | managed empty collection; delete all remote rulesets |
| `reconcile.rulesets: authoritative` + `spec.rulesets: [ ... ]` | exact-set management; delete remote rulesets not declared in YAML |

The same presence semantics apply to `labels` and `branch_protection`.

This lets RepositorySet defaults express organization-wide policy without
forcing every repository to manage every collection:

```yaml
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: my-org

defaults:
  reconcile:
    labels: authoritative
    rulesets: authoritative
    branch_protection: authoritative

repositories:
  - name: repo-a
    spec:
      rulesets:
        - name: protect-main
          target: branch
          rules:
            non_fast_forward: true

  - name: repo-b
    spec:
      description: "rulesets unmanaged for this repo"

  - name: repo-c
    spec:
      rulesets: []
```

In this example:

- `repo-a` manages rulesets authoritatively.
- `repo-b` does not manage rulesets.
- `repo-c` manages rulesets as an empty collection and deletes all remote rulesets.

This preserves gh-infra's core rule that omitted fields are unmanaged. Deletion
requires the target collection to be present in `spec`.

### File and FileSet Semantics

File and FileSet should use:

```yaml
reconcile: additive        # default; create/update declared files only
reconcile: authoritative   # manage the directory scope exactly; delete undeclared files
reconcile: create_only     # create if missing; do not update after creation
```

`authoritative` for files is scoped to the file entry's directory scope. It does
not mean the whole repository is authoritative unless the entry's scope covers
the whole repository.

### Legacy Values

File and FileSet currently accept:

| Old value | New value | Migration behavior |
|---|---|---|
| `patch` | `additive` | Deprecated alias |
| `mirror` | `authoritative` | Deprecated alias |
| `create_only` | `create_only` | No change |

The parser should continue accepting `patch` and `mirror` for compatibility,
normalize them to the new values internally, and emit deprecation warnings.

Repository's legacy label sync field remains separate:

```yaml
spec:
  label_sync: mirror
```

`label_sync: mirror` is a deprecated compatibility alias for
`reconcile.labels: authoritative`. It should continue to warn and should not be
used in new manifests.

## Rationale

### Use `authoritative` instead of `mirror`

`mirror` is short and intuitive, but in a GitHub repository management tool it
can be confused with repository mirroring or fork mirroring. `authoritative`
more accurately describes the behavior: YAML is the source of truth for the
managed scope, and undeclared remote entries may be deleted.

`authoritative` also matches common infrastructure-as-code terminology, such as
authoritative vs non-authoritative IAM resources in Terraform providers.

The word is longer, but destructive behavior benefits from explicit naming.

### Use `additive` instead of `patch`

`patch` is ambiguous in File resources because gh-infra also has a `patches:`
field for content-level patching. It also suggests JSON Patch or strategic merge
patch rather than the intended policy: create/update declared files while
leaving undeclared files untouched.

`additive` matches Repository terminology and better communicates the non-
destructive default behavior.

### Keep `create_only` under `reconcile`

`create_only` is partly a lifecycle policy rather than a pure reconciliation
policy. In theory, File entries could split this into separate fields:

```yaml
files:
  - path: config/app.yaml
    lifecycle: continuous
    reconcile: authoritative
```

We will not split it now. The practical combinations are effectively mutually
exclusive, and a second field would increase schema complexity with little user
benefit. `create_only` can be understood as "reconcile only on initial creation."

## Consequences

### Positive

- Repository and File/FileSet use the same words for the same reconciliation
  behaviors.
- `patches:` and `reconcile: patch` no longer compete for the word "patch."
- `authoritative` makes destructive behavior more explicit than `mirror`.
- Future resources can reuse the same vocabulary.

### Negative / Tradeoffs

- File/FileSet users need to learn new names.
- Existing manifests using `patch` or `mirror` need a migration path.
- `authoritative` may look like an ownership declaration. Documentation must
  clearly state that Repository collections are managed only when the
  corresponding `spec` collection is present.
- `authoritative` for files is scoped to a directory scope, not necessarily to
  the whole repository. Documentation must make the scope clear.

## Implementation Notes

- Add `ReconcileAdditive = "additive"` and
  `ReconcileAuthoritative = "authoritative"` for File/FileSet.
- Keep `ReconcileCreateOnly = "create_only"`.
- Accept legacy `patch` and `mirror` in YAML.
- Normalize legacy values to `additive` and `authoritative` during parsing.
- Emit deprecation warnings when legacy values are used.
- Update user documentation, examples, skill references, and tests to prefer
  the new values.
- Keep Repository `reconcile.<collection>` presence semantics unchanged:
  omitted `spec.<collection>` means unmanaged/no-op.

## Alternatives Considered

### Keep existing File names

Rejected. Keeping `patch` and `mirror` preserves compatibility but leaves
Repository and File/FileSet with inconsistent vocabulary. The inconsistency is
small today but becomes more expensive as more resources gain reconciliation
policy.

### Rename Repository to match File

Rejected. `mirror` is less precise in GitHub repository management, and `patch`
conflicts with File `patches:`.

### Split `create_only` into lifecycle

Rejected for now. It is conceptually cleaner but adds schema complexity and
invalid combinations without solving a current user problem.
