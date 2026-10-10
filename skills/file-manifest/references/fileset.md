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
- It names the FileSet in plan output and the default commit message, PR branch (`gh-infra/sync-<owner>-<name>`), and PR body
- Unnamed: named `<owner>/<repo-a>+<repo-b>` (sorted). Two unnamed FileSets listing the same repos share it, which is rejected when both use `via: pull_request`; set `name` or `branch`

## Overrides

- Match by `path`
- Replace the base entry for that repo only
- If override omits `vars`, base `vars` are inherited
- If override omits `patches`, base `patches` are inherited
- If override omits `executable`, base `executable` is inherited
- If override sets `patches`, it replaces the base patch list

Use `FileSet` for shared files across many repos. Use separate `File` resources when repos diverge heavily and need cleaner per-repo history.
