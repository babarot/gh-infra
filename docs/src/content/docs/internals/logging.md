---
title: Logging & Debugging
---

Set the log level via `GH_INFRA_LOG` environment variable or `--log-level` flag:

```bash
GH_INFRA_LOG=debug gh infra plan ./repos/
gh infra plan ./repos/ --log-level=trace
```

`--verbose` / `-V` is a shorthand for `--log-level=debug`.

## Log Levels

| Level | What it shows |
|-------|---------------|
| `error` | Fetch failures |
| `warn` | gh command failures with stderr |
| `info` | Fetch targets, plan summary |
| `debug` | Every gh command executed, response sizes, diff results |
| `trace` | Everything above + full API response bodies (stdout/stderr) |

## Example with trace

Useful for debugging API issues:

```
$ GH_INFRA_LOG=trace gh infra plan ./repos/

2026/03/21 03:03:04 INFO fetching repos=1 parallel=5
2026/03/21 03:03:04 DEBU exec cmd="gh repo view babarot/gh-infra --json ..."
2026/03/21 03:03:04 TRAC stdout cmd="gh repo view ..." output="{\"description\":\"...\", ...}"
2026/03/21 03:03:04 DEBU ok cmd="gh repo view ..." bytes=460
```
