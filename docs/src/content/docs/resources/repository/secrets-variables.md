---
title: Secrets & Variables
sidebar:
  order: 5
---

## Secrets

Secrets store sensitive values like API tokens, deploy keys, and webhook URLs. Values are referenced via environment variables to keep them out of YAML files.

```yaml
spec:
  secrets:
    - name: DEPLOY_TOKEN
      value: "${ENV_DEPLOY_TOKEN}"
    - name: SLACK_WEBHOOK
      value: "${ENV_SLACK_WEBHOOK}"
```

`${ENV_DEPLOY_TOKEN}` is resolved from the environment where `gh infra apply` runs — your terminal or CI environment.

### Limitations

GitHub does not expose secret values via the API. This means:

- **`plan` can detect new secrets** (ones that don't exist yet on GitHub), but it **cannot compare existing values**. Even if the value has changed, `plan` will show no diff.
- **To force-update secrets**, use the `--force-secrets` flag. This re-sets all secrets regardless of whether they've changed.

```bash
gh infra apply ./repos/ --force-secrets
```

This is useful after rotating credentials — without the flag, `apply` would skip existing secrets because it can't tell they've changed.

## Variables

Variables store non-sensitive configuration like environment names or regions.

```yaml
spec:
  variables:
    - name: APP_ENV
      value: production
    - name: REGION
      value: us-central1
```

Unlike secrets, variable values **are visible** via the API. `plan` can show the full diff when a value changes.
