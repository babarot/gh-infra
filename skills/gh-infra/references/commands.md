# Commands

## Global Flags

- `-V`, `--verbose`: shorthand for debug logging
- `--log-level <trace|debug|info|warn|error>`: explicit log level
- `--concurrency <n>` (or env `GH_INFRA_CONCURRENCY`): max `gh` processes at once across the whole command, default 8; lower it (even 1) on secondary rate limits or network timeouts
- rate limits are waited for, not failed on: secondary 1, 2, 4, 8 min; primary until the reset from `gh api rate_limit`; shared across all calls; at most 15 min per call, else `GitHub API rate limit exceeded; resets at ...`. Network errors retry 5 times (1-8s backoff)

## import

```bash
gh infra import <owner/repo> [owner/repo ...]
gh infra import <owner/repo> --into=<path>
gh infra import --into=<path> --auto-approve   # no prompt (CI, agents)
```

Use plain `import` to bootstrap manifests.

Use `--into` to pull live GitHub state back into existing manifests and local file sources. This is not the normal day-to-day workflow; use it when local YAML drifted behind GitHub and you want to reconcile local artifacts.

## validate

```bash
gh infra validate [path...]
```

- Checks YAML syntax and schema, plus checks across manifests (e.g. two `pull_request` FileSets sharing a branch on one repo)
- Does not contact GitHub
- Exits nonzero on validation failure
- Accepts multiple paths: `gh infra validate ./repos/ ./files/`

## plan

```bash
gh infra plan [path...]
```

Flags:

- `-r`, `--repo <owner/repo>`: limit to one repository
- `--detailed-exitcode`: exit 0 no changes, 1 error, 2 changes (prefer this in CI)
- `--ci`: deprecated (warns); exits 1 for changes and errors alike, disables colors. Use `--detailed-exitcode`, and `NO_COLOR=1` for plain output
- `--changed-since <ref>`: only manifests changed since the git ref (merge base to working tree, plus untracked) and File/FileSet docs whose local `source:` files/dirs or `patches:` files changed; no API calls when nothing changed. Needs the ref in the checkout (`fetch-depth: 0` in CI); every path must be inside the git repo; `github://` source changes are not detected. Nothing changed -> exit 0 (also with `--detailed-exitcode`); a bad ref (e.g. an all-zero `github.event.before`) -> exit 1. CI workflows: see the `ci-cd` skill
- `--fail-on-unknown`: fail on non-gh-infra YAML kinds instead of skipping
- `--diff`: print a unified diff for each FileSet file change (up to 500 lines per file; none for deletes)
- `-o`, `--output json`: print only a JSON document on stdout (summary, changes, warnings, errors); prefer it over parsing text. With `--changed-since` it adds `changed_since: {ref, selected, total}`; `selected: 0` means nothing was planned (not "no changes")
- `-o`, `--output markdown`: print only a pull request comment (summary, a collapsed table per repository, warnings, errors). With `--changed-since`: `Plan: no manifests changed since <ref>` when nothing was selected, otherwise a `Planned N of M resources ...` line

Exit status:

- exits 1 if any repository's current state could not be fetched (e.g. token lacks permissions), even with no changes; the failed repos are skipped, never planned as creates
- with `--detailed-exitcode`, exits 2 when changes are detected; an error (1) wins over changes; an interrupted plan or paths with no manifests also exit 1
- with the deprecated `--ci`, exits 1 when changes are detected

## apply

```bash
gh infra apply [path...]
```

Flags:

- `-r`, `--repo <owner/repo>`: limit to one repository
- `--auto-approve`: skip confirmation prompt
- `--force-secrets`: re-send all declared secrets
- `--changed-since <ref>`: same selection as `plan`; a failed run's changes are not retried by the next push, so keep a periodic full plan/apply
- `--fail-on-unknown`: fail on non-gh-infra YAML kinds instead of skipping
- `-o`, `--output json`: print only a JSON document with a `status` per change; requires `--auto-approve`

Exit status: 1 if any change fails or any repository could not be fetched (those repos are skipped; the rest are still applied).

Interactive diff viewer:

- `d`: open diff viewer from confirm prompt
- `j` / `k` or arrows: move selection
- `Tab`: toggle apply/skip for selected file
- `d` / `u`: scroll diff
- `q` / `Esc`: return to confirmation

Apply/skip choices are runtime-only. They do not rewrite the manifest.
