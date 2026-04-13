# Security

Fields under this section correspond to GitHub's **Advanced Security** settings page.

All Advanced Security fields live under the `security` map.

## Vulnerability Alerts

```yaml
spec:
  security:
    vulnerability_alerts: true
```

Enable Dependabot vulnerability alerts. Required when integrating with tools like Renovate's `osvVulnerabilityAlerts`. Uses PUT/DELETE on `/repos/{owner}/{repo}/vulnerability-alerts`; GitHub reports state via HTTP status (204 = enabled, 404 = disabled).
