---
title: Security
sidebar:
  order: 8
---

Security settings correspond to the **Advanced Security** section of the GitHub repository settings UI. They are grouped under the `security` key on the repository spec:

```yaml
spec:
  security:
    vulnerability_alerts: true
```

## Vulnerability Alerts

Enable or disable Dependabot vulnerability alerts for the repository:

```yaml
spec:
  security:
    vulnerability_alerts: true
```

| Field | Type | Description |
|-------|------|-------------|
| `security.vulnerability_alerts` | bool | `true` to enable Dependabot vulnerability alerts. Required for features like Renovate's `osvVulnerabilityAlerts` that integrate with Dependabot |

This setting uses a dedicated GitHub API endpoint (`/repos/{owner}/{repo}/vulnerability-alerts`) rather than the standard repository settings endpoint. The API signals state via HTTP status: `204 No Content` when enabled, `404 Not Found` when disabled.
