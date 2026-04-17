# FileSet

## Shape

```yaml
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: my-org
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

## Overrides

- Match by `path`
- Replace the base entry for that repo only
- If override omits `vars`, base `vars` are inherited
- If override omits `patches`, base `patches` are inherited
- If override sets `patches`, it replaces the base patch list

Use `FileSet` for shared files across many repos. Use separate `File` resources when repos diverge heavily and need cleaner per-repo history.
