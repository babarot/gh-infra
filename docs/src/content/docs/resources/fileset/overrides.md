---
title: Per-repo Overrides
---

Each repository in a `FileSet` receives the same set of files. Overrides let you customize specific files for specific repos while keeping the rest consistent.

```yaml
spec:
  repositories:
    - gomi
    - name: gh-infra
      overrides:
        - path: .github/CODEOWNERS
          content: |
            * @babarot @co-maintainer

  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot
    - path: LICENSE
      source: ./templates/LICENSE
```

In this example, `gomi` gets the default CODEOWNERS (`* @babarot`), while `gh-infra` gets a customized version with an additional maintainer. Both repos receive the same LICENSE.

## How Overrides Work

An override replaces the matching file entry (matched by `path`) for that repository only. The override must specify `path` and either `content` or `source`, just like a regular file entry.

Files without a matching override are unchanged — they use the default from the `files` list.

## Vars Inheritance

If an override doesn't specify `vars`, it inherits the `vars` from the original file entry:

```yaml
spec:
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
        binary_name: "<% .Repo.Name %>"     # default for all repos
```

- `gomi` uses `binary_name: "gomi"` (from the template expansion of `<% .Repo.Name %>`)
- `special-repo` uses `binary_name: "special-binary"` (from the override)

## on_drift Inheritance

Like `vars`, `on_drift` is inherited from the original file entry if the override doesn't specify its own:

```yaml
spec:
  repositories:
    - gomi
    - name: special-repo
      overrides:
        - path: .gitignore
          content: "/special-binary\n"
          on_drift: skip           # override drift handling for this repo

  files:
    - path: .gitignore
      on_drift: overwrite          # default for all repos
      content: "/gomi\n"
```

- `gomi` uses `on_drift: overwrite` (from the file entry)
- `special-repo` uses `on_drift: skip` (from the override)
