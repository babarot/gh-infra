---
title: Per-repo Overrides
---

Each repository in a `FileSet` receives the same set of files. Overrides let you customize specific files for specific repos while keeping the rest consistent.

```yaml
spec:
  repositories:
    - my-cli
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

In this example, `my-cli` gets the default CODEOWNERS (`* @babarot`), while `gh-infra` gets a customized version with an additional maintainer. Both repos receive the same LICENSE.

## How Overrides Work

An override replaces the matching file entry (matched by `path`) for that repository only. A field the override leaves out is taken from the original entry, including `content` and `source`, so an override can change a single field such as `vars` or `description`.

Files without a matching override are unchanged — they use the default from the `files` list.

An override must match an entry: one whose `path` is in no entry of `files`, or two overrides with the same `path` for one repository, make `validate`, `plan` and `apply` fail.

## Directory Entries

For an entry with a directory source, the override's `path` is the entry's `path`, the directory:

```yaml
spec:
  repositories:
    - name: api
      overrides:
        # Change fields of every file in the directory
        - path: .regomark/policy
          description: Policies for the API's ADRs.
    - name: web
      overrides:
        # Replace the directory: web gets ./web-policy/ instead
        - path: .regomark/policy
          source: ./web-policy/

  files:
    - path: .regomark/policy
      source: ./policy/
      reconcile: authoritative
```

- Without `content` or `source`, the override sets its fields (`vars`, `patches`, `reconcile`, `executable`, `description`) on every file of the directory. Each file keeps its own content.
- With a directory source, the override replaces the whole directory: the repository gets the files of the override's directory and none of the original's. Fields the override leaves out come from the original entry. With `reconcile: authoritative`, a file only the original directory has is deleted from the repository; with `additive`, it is left there.
- An override can also replace one file of the directory by its full path (`path: .regomark/policy/links.rego`). It applies after an override of the directory's fields, and cannot be combined with replacing the directory: change the file in the replacing directory instead.
- A directory source ends in `/`. The override's directory must have files, and its files cannot share a path with another entry. A `content`, or a single-file source, for a directory entry is an error, as is a directory source for a file entry.

## Vars Inheritance

If an override doesn't specify `vars`, it inherits the `vars` from the original file entry:

```yaml
spec:
  repositories:
    - my-cli
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

- `my-cli` uses `binary_name: "my-cli"` (from the template expansion of `<% .Repo.Name %>`)
- `special-repo` uses `binary_name: "special-binary"` (from the override)

`executable` is inherited the same way: an override without `executable` uses the value from the original file entry.

## Description

`description` is inherited the same way. Override it to tell one repository something the others do not need, for example that the shared file replaces one of its own:

```yaml
spec:
  repositories:
    - claude-recall
    - name: drop
      overrides:
        - path: .claude/rules/adr.md
          description: >-
            Replaces this repository's own ADR rule with the shared one,
            so its ADRs follow the same rules as the other repositories.

  files:
    - path: .claude/rules/adr.md
      source: ./templates/.claude/rules/adr.md
      description: The rules for writing ADRs in docs/adr.
```

See [Pull request description](../../file/delivery/#pull-request-description).

