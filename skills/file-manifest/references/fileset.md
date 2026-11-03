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
- An override whose `path` matches no entry, or two overrides with the same `path` for one repo, fail `validate`/`plan`/`apply`
- Directory entry (`path: policy, source: ./policy/`): an override with `path: policy` and no `content`/`source` sets its fields on every file (each keeps its content); with `source: ./other/` (trailing `/`) it replaces the whole directory for that repo (files only the original has are not delivered; deleted from the repo only with `reconcile: authoritative`)
- A file inside a directory entry can be overridden by its full path (`path: policy/a.rego`); applied after a directory-fields override. Cannot be combined with replacing the same directory (parse fails on a duplicate path); edit the replacing directory instead
- Errors: `content` or a single-file source on a directory entry; a directory source on a file entry; an override directory with no files; a replacing directory bringing a path another entry also writes
- If override sets `patches`, it replaces the base patch list

Use `FileSet` for shared files across many repos. Use separate `File` resources when repos diverge heavily and need cleaner per-repo history.
