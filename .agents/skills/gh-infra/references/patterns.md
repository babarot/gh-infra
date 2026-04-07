# Patterns

## Central Management

Use a dedicated repository when one person or team manages many repositories.

Typical layout:

```text
github-config/
  repos/
  files/
```

Use `Repository` / `RepositorySet` under `repos/` and `FileSet` under `files/`.

## Self-Managed

Use a per-repository manifest when each team owns its own settings.

Typical layout:

```text
my-repo/
  .github/
    infra.yaml
    workflows/
      infra.yaml
```

Run `gh infra apply .github/infra.yaml --auto-approve` on merge to `main`.

## Choosing

Prefer central management when:

- you need org-wide consistency
- one team owns governance
- you want one audit trail

Prefer self-managed when:

- teams own their own repository settings
- infra changes should be reviewed alongside code
