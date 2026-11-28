---
title: Security
sidebar:
  order: 9
---

Security settings correspond to the **Advanced Security** section of the GitHub repository settings UI. They are grouped under the `security` key on the repository spec:

```yaml
spec:
  security:
    vulnerability_alerts: true
    automated_security_fixes: true
    private_vulnerability_reporting: true
    secret_scanning: true
    secret_scanning_push_protection: true
```

| Field | Type | Description |
|-------|------|-------------|
| `vulnerability_alerts` | bool | Enable Dependabot vulnerability alerts |
| `automated_security_fixes` | bool | Enable Dependabot security updates (auto-PRs that fix vulnerabilities). Requires `vulnerability_alerts: true` on GitHub side |
| `private_vulnerability_reporting` | bool | Allow security researchers to privately report vulnerabilities |
| `secret_scanning` | bool | Enable secret scanning, which alerts on secrets committed to the repository |
| `secret_scanning_push_protection` | bool | Enable push protection, which blocks a push that contains a supported secret. Requires `secret_scanning` to be enabled |

### Field dependencies

`automated_security_fixes` requires `vulnerability_alerts` to be effectively enabled (per GitHub's API). gh-infra evaluates this at `plan` time using both the manifest and the current GitHub state, and rejects the plan if the dependency cannot be satisfied:

- If `vulnerability_alerts: true` is set in the same manifest, the plan succeeds and `apply` enables alerts before fixes (ordering is guaranteed).
- If `vulnerability_alerts` is omitted from the manifest, gh-infra falls back to the repository's current GitHub state (omission means "leave as-is"). The plan succeeds only when alerts are already enabled on GitHub.
- If `vulnerability_alerts: false` is set explicitly, the plan fails with a clear error.

`secret_scanning_push_protection: true` requires `secret_scanning` to be effectively enabled, by the same rule: the manifest's value when it sets one, otherwise the repository's current state. A plan that cannot satisfy it fails with an error. `apply` enables secret scanning before push protection.

`private_vulnerability_reporting` has no dependency on the other fields and can be toggled independently.

## Vulnerability Alerts

Enable or disable Dependabot vulnerability alerts:

```yaml
spec:
  security:
    vulnerability_alerts: true
```

Required for tools like Renovate's `osvVulnerabilityAlerts` that integrate with Dependabot.

## Automated Security Fixes

Enable or disable Dependabot security updates — automated pull requests that resolve open Dependabot alerts:

```yaml
spec:
  security:
    automated_security_fixes: true
```

Requires `vulnerability_alerts` to be effectively enabled — see [Field dependencies](#field-dependencies) above.

## Private Vulnerability Reporting

Allow security researchers to privately report vulnerabilities through GitHub's security advisory workflow:

```yaml
spec:
  security:
    private_vulnerability_reporting: true
```

## Secret Scanning

Alert on secrets committed to the repository:

```yaml
spec:
  security:
    secret_scanning: true
```

Secret scanning is always on for a public repository, so `secret_scanning: false` there is not applied: `apply` reports it as failed. A private repository needs GitHub Secret Protection (or GitHub Advanced Security) for it.

Both secret scanning settings are read from the `security_and_analysis` object of the repository, which GitHub returns only to users who can administer it. When it is not returned, `plan` reports that the settings cannot be read and leaves the repository out, instead of comparing them as disabled.

## Push Protection

Block a push that contains a supported secret:

```yaml
spec:
  security:
    secret_scanning: true
    secret_scanning_push_protection: true
```

Requires `secret_scanning` to be effectively enabled — see [Field dependencies](#field-dependencies) above. Disabling `secret_scanning` disables push protection with it on GitHub.
