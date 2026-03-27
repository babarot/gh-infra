---
paths:
  - "**/*.go"
---

# Go Identifier Renaming

When renaming Go types, functions, methods, or struct fields, use `gopls rename` instead of sed/perl/manual editing.

```bash
gopls rename -w <file>:<line>:<column> <NewName>
```

Rules:
- **Always use `gopls rename -w`** for any exported identifier rename (types, functions, methods, fields)
- **Point at the definition site**, not a usage site — gopls resolves all references across packages automatically
- **Line and column are 1-based** — match what editors and `grep -n` show
- **Verify after each rename** with `go build ./...` before proceeding to the next rename
- **Never use sed/perl** for Go identifier renames — they don't understand Go semantics, miss word boundaries, and break on macOS BSD sed
- Unexported (lowercase) identifiers can also be renamed with gopls if they have cross-file references within the same package
