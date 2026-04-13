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
    automated_security_fixes: true
    private_vulnerability_reporting: true
```

| Field | Type | Description |
|-------|------|-------------|
| `vulnerability_alerts` | bool | Enable Dependabot vulnerability alerts |
| `automated_security_fixes` | bool | Enable Dependabot security updates (auto-PRs that fix vulnerabilities). Requires `vulnerability_alerts: true` on GitHub side |
| `private_vulnerability_reporting` | bool | Allow security researchers to privately report vulnerabilities |

Each field is optional and uses a dedicated GitHub REST API endpoint (PUT to enable, DELETE to disable).

### Field dependencies

`automated_security_fixes` requires `vulnerability_alerts` to be effectively enabled (per GitHub's API). gh-infra evaluates this at `plan` time using both the manifest and the current GitHub state, and rejects the plan if the dependency cannot be satisfied:

- If `vulnerability_alerts: true` is set in the same manifest, the plan succeeds and `apply` enables alerts before fixes (ordering is guaranteed).
- If `vulnerability_alerts` is omitted from the manifest, gh-infra falls back to the repository's current GitHub state (omission means "leave as-is"). The plan succeeds only when alerts are already enabled on GitHub.
- If `vulnerability_alerts: false` is set explicitly, the plan fails with a clear error.

`private_vulnerability_reporting` has no dependency on the other fields and can be toggled independently.

## Vulnerability Alerts

Enable or disable Dependabot vulnerability alerts:

```yaml
spec:
  security:
    vulnerability_alerts: true
```

Endpoint: `/repos/{owner}/{repo}/vulnerability-alerts`. Required for features like Renovate's `osvVulnerabilityAlerts` that integrate with Dependabot. The API signals state via HTTP status: `204 No Content` when enabled, `404 Not Found` when disabled.

## Automated Security Fixes

Enable or disable Dependabot security updates — automated pull requests that resolve open Dependabot alerts:

```yaml
spec:
  security:
    automated_security_fixes: true
```

Endpoint: `/repos/{owner}/{repo}/automated-security-fixes`. Requires `vulnerability_alerts` to be effectively enabled — see [Field dependencies](#field-dependencies) above.

## Private Vulnerability Reporting

Allow security researchers to privately report vulnerabilities through GitHub's security advisory workflow:

```yaml
spec:
  security:
    private_vulnerability_reporting: true
```

Endpoint: `/repos/{owner}/{repo}/private-vulnerability-reporting`.
