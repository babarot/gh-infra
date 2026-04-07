# Secrets And Variables

## Secrets

```yaml
spec:
  secrets:
    - name: DEPLOY_TOKEN
      value: "${ENV_DEPLOY_TOKEN}"
```

Rules:

- never put literal secret values in YAML
- use `${ENV_*}` references only
- `plan` cannot diff existing secret values
- use `gh infra apply --force-secrets` after secret rotation

## Variables

```yaml
spec:
  variables:
    - name: APP_ENV
      value: production
```

Variables are plain text and fully diffable in `plan`.
