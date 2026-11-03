# FileSet

## Shape

```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: my-org
  name: shared-ci     # optional
spec:
  repositories:
    - repo-a
    - name: repo-b
      overrides:
        - path: .github/CODEOWNERS
          content: |
            * @team-b
  files:
    - path: .github/CODEOWNERS
      content: |
        * @username
```

## Name

- `metadata.name` is optional and does not select repos
- It names the FileSet in plan output and the default PR branch (`gh-infra/sync-<owner>-<name>`)
- Unnamed: named `<owner>/<repo-a>+<repo-b>` (sorted). Two unnamed FileSets listing the same repos share it, which is rejected when both use `via: pull_request`; set `name` or `branch`

## Overrides

- Match by `path`
- Replace the base entry for that repo only
- If override omits `vars`, base `vars` are inherited
- If override omits `patches`, base `patches` are inherited
- If override omits `executable`, base `executable` is inherited
- If override omits `description`, base `description` is inherited
- If override omits `content` and `source`, the base content/source is inherited, so `path` + `description` alone changes only that repo's PR description
- An override matches a single file's `path`; it does NOT match an entry with a directory source (`path: .regomark/policy` in overrides is silently ignored)
- If override sets `patches`, it replaces the base patch list

Use `FileSet` for shared files across many repos. Use separate `File` resources when repos diverge heavily and need cleaner per-repo history.
