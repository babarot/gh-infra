# Patches

Use `patches` when a repo mostly follows a shared template but needs small local deltas.

```yaml
files:
  - path: .gitignore
    source: ./templates/common/.gitignore
    patches:
      - ./patches/frontend-gitignore.patch
```

## Rules

- Each patch entry can be a file path or inline diff content
- Prefer file paths; they preserve tabs and are easier to review
- Patches apply after template expansion
- Multiple patches apply in order

Processing order:

```text
source/content -> template expansion -> patches -> sync
```

## Use Patches When

- a file is mostly shared
- only a few lines differ per repo
- you want upstream template updates to keep flowing through

Use a separate source file instead when the patch is large or the target diverges heavily.
