# Security

Fields under this section correspond to GitHub's **Advanced Security** settings page.

All Advanced Security fields live under the `security` map.

```yaml
spec:
  security:
    vulnerability_alerts: true
    automated_security_fixes: true
    private_vulnerability_reporting: true
```

| Field | Endpoint | Notes |
|---|---|---|
| `vulnerability_alerts` | `vulnerability-alerts` | 204 = enabled / 404 = disabled |
| `automated_security_fixes` | `automated-security-fixes` | Requires `vulnerability_alerts: true` (server-side check) |
| `private_vulnerability_reporting` | `private-vulnerability-reporting` | |

All three use PUT to enable, DELETE to disable on `/repos/{owner}/{repo}/<endpoint>`.

Validation traps:
- `automated_security_fixes` requires `vulnerability_alerts` to be effectively true. Checked at `plan` (NOT `validate` — the check needs current GitHub state). Effective state = manifest value if set, else current remote value (omission = unmanaged).
- `vulnerability_alerts: false` + `automated_security_fixes: true` → plan fails with explicit error.
- `automated_security_fixes: true` alone (alerts omitted) → plan succeeds only if remote alerts already true; otherwise fails with same error.
- Apply order: when both are set true in one manifest, `vulnerability_alerts` is sent before `automated_security_fixes` (guaranteed by `diffSecurity` child order and `applyAllSettings` for new repos).
- `private_vulnerability_reporting` is independent — no dependency.
