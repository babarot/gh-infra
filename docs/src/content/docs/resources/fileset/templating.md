---
title: Templating
---

File content can include Go template expressions that are expanded per target repository. This lets you share a single template across repos while customizing values like project names, module paths, and binary names.

## Quick Example

```yaml
files:
  - path: Makefile
    source: ./templates/Makefile

  - path: go.mod
    content: |
      module github.com/<% .Repo.FullName %>
      go 1.24.0
```

When applied to `babarot/gomi`, the `go.mod` becomes:

```
module github.com/babarot/gomi
go 1.24.0
```

## Built-in Variables

Every template has access to the target repository's metadata:

| Variable | Value | Example (`babarot/gomi`) |
|---|---|---|
| `<% .Repo.Name %>` | Repository name | `gomi` |
| `<% .Repo.Owner %>` | Owner name | `babarot` |
| `<% .Repo.FullName %>` | `owner/name` | `babarot/gomi` |

## Custom Variables

Use `vars` to define additional variables. Values can themselves reference built-in variables:

```yaml
files:
  - path: .github/workflows/release.yml
    source: ./templates/release.yml
    vars:
      binary_name: "<% .Repo.Name %>"
      docker_image: "ghcr.io/<% .Repo.FullName %>"
```

In the template file:

```yaml
- run: docker build -t <% .Vars.docker_image %> .
- run: ./<% .Vars.binary_name %> --version
```

Variables are resolved in two passes:
1. `vars` values are expanded (can reference `.Repo` only)
2. File content is expanded (can reference both `.Repo` and `.Vars`)

## Compatibility with Other Template Systems

gh-infra uses `<% %>` delimiters, which do not conflict with:

- **GitHub Actions** `${{ }}` — preserved as-is
- **GoReleaser** `{{ .Version }}` — preserved as-is
- **Helm** `{{ .Release.Name }}` — preserved as-is
- **Any `{{ }}` syntax** — untouched by gh-infra

```yaml
files:
  - path: .github/workflows/ci.yml
    content: |
      runs-on: ${{ matrix.os }}              # ← GitHub Actions (no conflict)
      env:
        TOKEN: ${{ secrets.GITHUB_TOKEN }}   # ← GitHub Actions (no conflict)
      steps:
        - run: echo "Building <% .Repo.Name %>"  # ← gh-infra (expanded)
```

No escaping needed.

## When Templates Are Applied

Templates are expanded during `plan`, before the diff comparison. This means:

- `plan` output shows the **expanded** content
- `apply` commits the **expanded** content
- Each target repository gets its own expansion with the correct `.Repo` values

Files without `<% %>` syntax are **not processed** by the template engine — they pass through unchanged.

## Error Handling

If a template references an undefined variable, `plan` and `apply` stop with an error:

```yaml
files:
  - path: Makefile
    content: |
      BINARY = <% .Vars.typo %>   # ← error: undefined key "typo"
```

```
Error: template Makefile for babarot/gomi: template: :1:12:
  executing "" at <.Vars.typo>: map has no entry for key "typo"
```

This prevents accidentally committing files with missing values.

## Per-repo Overrides with Vars

Overrides can define their own `vars` for specific repos. If an override doesn't specify `vars`, it inherits from the original file entry:

```yaml
repositories:
  - gomi
  - name: special-repo
    overrides:
      - path: Makefile
        vars:
          binary_name: "special-binary"   # overrides the default

files:
  - path: Makefile
    source: ./templates/Makefile
    vars:
      binary_name: "<% .Repo.Name %>"     # default
```
