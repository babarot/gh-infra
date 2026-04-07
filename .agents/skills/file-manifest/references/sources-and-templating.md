# Sources And Templating

## Source Types

Inline content:

```yaml
files:
  - path: .github/CODEOWNERS
    content: |
      * @username
```

Local file:

```yaml
files:
  - path: LICENSE
    source: ./templates/LICENSE
```

Local directory:

```yaml
files:
  - path: .github/workflows
    source: ./templates/workflows/
```

GitHub source:

```text
github://<owner>/<repo>/<path>
github://<owner>/<repo>/<path>/
github://<owner>/<repo>/<path>@<ref>
```

Pin GitHub sources in production.

## Templating

gh-infra uses `<% %>`.

Built-in variables:

- `<% .Repo.Name %>`
- `<% .Repo.Owner %>`
- `<% .Repo.FullName %>`

Custom vars:

```yaml
files:
  - path: Makefile
    source: ./templates/Makefile
    vars:
      binary_name: "<% .Repo.Name %>"
```

Two-pass expansion:

1. `vars` values may reference `.Repo`
2. file content may reference `.Repo` and `.Vars`

Undefined variables are errors.
