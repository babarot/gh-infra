---
paths:
  - ".github/workflows/**"
---

# GitHub Actions Workflow Rules

After editing workflow files, always run the following checks and fix any issues before considering the task complete.

## pinact — SHA pinning

[github.com/suzuki-shunsuke/pinact](https://github.com/suzuki-shunsuke/pinact)

Pin third-party actions to full-length commit SHAs to mitigate supply chain attacks. The `-u` flag updates actions to their latest stable version before pinning. `PINACT_MIN_AGE=7` skips releases younger than 7 days as a cooldown window.

```bash
PINACT_MIN_AGE=7 pinact run -u
```

## ghalint — security policy linter

[github.com/suzuki-shunsuke/ghalint](https://github.com/suzuki-shunsuke/ghalint)

Enforces security best practices: explicit `permissions` on every job, `timeout-minutes`, `persist-credentials: false` on checkout steps, and scoped secrets. Legitimate exceptions go in `.ghalint.yaml` with a comment explaining why.

```bash
ghalint run
```

## actionlint — workflow linter

[github.com/rhysd/actionlint](https://github.com/rhysd/actionlint)

Catches syntax errors, type mismatches, and shellcheck issues in workflow files.

```bash
actionlint .github/workflows/*.yaml
```
