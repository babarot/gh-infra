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
| `secret_scanning` | bool | Enable secret scanning |
| `secret_scanning_push_protection` | bool | Enable push protection for the repository, which blocks pushes that contain secrets. Requires secret scanning |

### Field dependencies

`automated_security_fixes` requires `vulnerability_alerts` to be effectively enabled (per GitHub's API). gh-infra evaluates this at `plan` time using both the manifest and the current GitHub state, and rejects the plan if the dependency cannot be satisfied:

- If `vulnerability_alerts: true` is set in the same manifest, the plan succeeds and `apply` enables alerts before fixes (ordering is guaranteed).
- If `vulnerability_alerts` is omitted from the manifest, gh-infra falls back to the repository's current GitHub state (omission means "leave as-is"). The plan succeeds only when alerts are already enabled on GitHub.
- If `vulnerability_alerts: false` is set explicitly, the plan fails with a clear error.

`secret_scanning_push_protection` requires `secret_scanning` to be effectively enabled. GitHub does not reject a request that breaks this: it does not save push protection while secret scanning is off, and disabling secret scanning also disables push protection. gh-infra checks it the same way, at `plan` time:

- `secret_scanning_push_protection: true` with `secret_scanning: false` fails `validate`.
- If `secret_scanning` is omitted, `secret_scanning_push_protection: true` succeeds only when secret scanning is already enabled on GitHub.
- If `secret_scanning_push_protection` is omitted, `secret_scanning: false` fails while push protection is enabled on GitHub: set `secret_scanning_push_protection: false` as well.
- For a repository that the run creates, how it starts is not known (GitHub creates a public repository of a user with both enabled, and an organization's security configuration may set them either way), so set both fields: `secret_scanning_push_protection: true` needs `secret_scanning: true`, and `secret_scanning: false` needs `secret_scanning_push_protection: false`.

`apply` enables secret scanning before push protection, and disables push protection before secret scanning.

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

Enable secret scanning, and push protection, which blocks pushes that contain secrets:

```yaml
spec:
  security:
    secret_scanning: true
    secret_scanning_push_protection: true
```

Push protection requires secret scanning — see [Field dependencies](#field-dependencies) above. The field is the repository's push protection setting; push protection for users, which GitHub sets per account, is not managed here.

Secret scanning is free on public repositories. On a private or internal repository owned by an organization it needs [GitHub Secret Protection](https://docs.github.com/en/code-security/secret-scanning/introduction/about-secret-scanning), a paid product on GitHub Team and GitHub Enterprise Cloud; on a private repository owned by a user, GitHub offers it only on GitHub Enterprise Cloud with Enterprise Managed Users.

GitHub returns these settings only to repository admins, organization owners and security managers:

- **Not offered** (a repository that is not public, read by an admin, where GitHub leaves them out): the settings are off. `plan` shows no change for `false`, and fails for `true`; for a repository the same plan makes public, apply the visibility change first. `import` leaves both fields out, and `import --into` keeps `false` and removes `true` (in a RepositorySet entry whose defaults enable them, it sets them to `false`).
- **Not readable** (read without admin access, or left out on a public repository): `plan` reports that the settings cannot be read and leaves the repository out. `import` leaves both fields out, and `import --into` keeps the manifest's values and warns that they could not be read.

Making a public repository private or internal can take secret scanning away, depending on GitHub Secret Protection, which `plan` cannot see. So `plan` fails when the same plan enables either setting, or keeps it enabled: leave both fields out for the visibility change, then set them again. Disabling them in the same plan works, as `apply` sends it before the visibility change.

A public repository that the run creates starts with both settings on, and `apply` sends the manifest's values, `false` included. For a repository that is not public and that the run creates, `plan` notes that enabling secret scanning fails if GitHub does not offer it there, and that `false` is not sent, so if the owner's security configuration turns secret scanning on, the next plan shows it.
