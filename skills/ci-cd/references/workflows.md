# Recommended Workflows

The same workflows as docs `patterns/ci-cd.md`. Paths `./repos/ ./files/` and secret `GH_INFRA_TOKEN` are placeholders; for self-managed use `.github/infra.yaml`.

## Plan on pull request (comment, fail only on errors)

```yaml
# .github/workflows/infra-plan.yaml
on:
  pull_request:
permissions:
  contents: read
  pull-requests: write
jobs:
  plan:
    # PRs from forks get no secrets and a read-only GITHUB_TOKEN; skip them.
    if: github.event.pull_request.head.repo.full_name == github.repository
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # --changed-since needs the history of the base branch
      - run: gh extension install babarot/gh-infra
      - id: plan
        run: |
          gh infra plan ./repos/ ./files/ \
            --changed-since origin/${{ github.base_ref }} \
            --detailed-exitcode -o markdown > plan.md || echo "code=$?" >> "$GITHUB_OUTPUT"
      - if: ${{ !cancelled() }}
        env:
          GH_TOKEN: ${{ github.token }}   # comment as github-actions[bot]
        run: |
          [ -s plan.md ] || exit 0   # the plan failed before writing anything
          gh pr comment ${{ github.event.pull_request.number }} \
            --body-file plan.md --edit-last --create-if-none
      - if: steps.plan.outputs.code == '1'
        run: exit 1   # the plan itself failed
```

## Apply on merge (only what the push changed)

```yaml
# .github/workflows/infra-apply.yaml
on:
  push:
    branches: [main]
  workflow_dispatch:
jobs:
  apply:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
      GH_INFRA_SOURCE_URL: ${{ github.event.head_commit.url }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - run: gh extension install babarot/gh-infra
      - env:
          BEFORE: ${{ github.event.before }}
        run: |
          # A manual run, or the first push of a branch, has no previous
          # commit to compare with: apply everything.
          if [ -z "$BEFORE" ] || [ "$BEFORE" = "0000000000000000000000000000000000000000" ]; then
            gh infra apply ./repos/ ./files/ --auto-approve
          else
            gh infra apply ./repos/ ./files/ --changed-since "$BEFORE" --auto-approve
          fi
```

## Drift detection (everything, notify on drift)

```yaml
# .github/workflows/infra-drift.yaml
on:
  schedule:
    - cron: "0 9 * * 1"   # every Monday at 9am UTC
  workflow_dispatch:
permissions:
  contents: read
  issues: write
jobs:
  drift:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - id: plan
        run: gh infra plan ./repos/ ./files/ --detailed-exitcode -o markdown > plan.md || echo "code=$?" >> "$GITHUB_OUTPUT"
      - if: steps.plan.outputs.code == '2'
        env:
          GH_TOKEN: ${{ github.token }}
        run: gh issue create --title "Drift detected by gh-infra" --body-file plan.md
      - if: steps.plan.outputs.code == '1'
        run: exit 1
```

Replace the `gh issue create` step with any notification (Slack, email). To keep one open issue instead of a new one each week, look it up first with `gh issue list --search "Drift detected by gh-infra in:title" --state open`.

## Import changes made on GitHub (optional, opens a pull request)

```yaml
# .github/workflows/infra-import.yaml
on:
  schedule:
    - cron: "0 9 * * 1"
  workflow_dispatch:
permissions:
  contents: write
  pull-requests: write
jobs:
  import:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ secrets.GH_INFRA_TOKEN }}
    steps:
      - uses: actions/checkout@v4
      - run: gh extension install babarot/gh-infra
      - run: gh infra import --into=./repos/ --auto-approve
      - run: |
          [ -n "$(git status --porcelain)" ] || exit 0   # nothing to import
          branch="gh-infra/import-$(date +%Y%m%d%H%M)"
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          git checkout -b "$branch"
          git add -A
          git commit -m "Import GitHub state"
          git push -u origin "$branch"
          gh pr create --head "$branch" --title "Import GitHub state" --body "Changes made on GitHub, pulled into the manifests by gh infra import --into."
```

- `import --into` only edits the checkout; the last step commits and opens a pull request. `--into` takes one path, so run it once per manifest directory
- it reads the same settings as `plan`, so it needs the same token
- open the pull request with the PAT/App token (as here), not `github.token`: a pull request created with `GITHUB_TOKEN` does not trigger the plan workflow
