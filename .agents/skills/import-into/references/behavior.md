# Behavior

## Command Examples

```bash
gh infra import my-org/my-project --into=repos/my-project.yaml
gh infra import my-org/my-project --into=repos/
gh infra import my-org/my-project my-org/my-cli --into=repos/
```

## Resources Affected

- `Repository`: field-by-field YAML patch
- `RepositorySet`: preserve defaults vs per-repo overrides where possible
- inline `File` / `FileSet` entries: update manifest `content: |`
- local-source file entries: update the local source file

## Interactive Viewer

When file changes exist, the import flow uses a diff viewer.

Keys:

- `j` / `k` or arrows: select file
- `Tab`: cycle `write` / `patch` / `skip`
- `d` / `u`: scroll diff
- `q` / `Esc`: return to confirmation

Repository setting changes are shown in normal plan output, not the diff viewer.
