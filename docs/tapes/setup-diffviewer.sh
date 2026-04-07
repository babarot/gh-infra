#!/usr/bin/env bash
# Setup for diff viewer demo: FileSet with file changes across 2 repos.
set -euo pipefail

cp /data/.gh-infra /usr/local/bin/gh-infra
chmod +x /usr/local/bin/gh-infra

cat > /usr/local/bin/gh << 'WRAPPER'
#!/usr/bin/env bash
if [[ "$1" == "infra" ]]; then
  shift
  exec /usr/local/bin/gh-infra "$@"
fi
exec /data/mock-gh "$@"
WRAPPER
chmod +x /usr/local/bin/gh

export MOCK_DIR=/tmp/mock-data

for repo in app-api app-web; do
  dir="$MOCK_DIR/babarot/${repo}"
  mkdir -p "$dir" "$dir/contents/.github/workflows"

  # Repo settings (no changes — just needed so plan doesn't error)
  cat > "$dir/view.json" << 'JSON'
{
  "description": "",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": true,
  "hasWikiEnabled": false,
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": true,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": true,
  "defaultBranchRef": { "name": "main" }
}
JSON

  # Old CODEOWNERS
  echo -n '* @old-owner' > "$dir/contents/.github/CODEOWNERS"

  # Old CI workflow (checkout v3, push only, no lint, no coverage)
  cat > "$dir/contents/.github/workflows/ci.yml" << 'CI'
name: CI
on: [push]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version-file: go.mod

      - name: Test
        run: make test
CI

  # dependabot.yml does NOT exist on GitHub (404 → new file)
done

mkdir -p /tmp/demo

cat > /tmp/demo/files.yaml << 'YAML'
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot

spec:
  repositories:
    - app-api
    - app-web

  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot @team-platform

    - path: .github/workflows/ci.yml
      content: |
        name: CI
        on:
          push:
            branches: [main]
          pull_request:

        jobs:
          lint:
            runs-on: ubuntu-latest
            steps:
              - uses: actions/checkout@v4

              - name: Setup Go
                uses: actions/setup-go@v5
                with:
                  go-version-file: go.mod

              - name: Lint
                uses: golangci/golangci-lint-action@v6

          test:
            runs-on: ubuntu-latest
            steps:
              - uses: actions/checkout@v4

              - name: Setup Go
                uses: actions/setup-go@v5
                with:
                  go-version-file: go.mod

              - name: Test
                run: make test

              - name: Upload coverage
                uses: codecov/codecov-action@v4

    - path: .github/dependabot.yml
      content: |
        version: 2
        updates:
          - package-ecosystem: gomod
            directory: /
            schedule:
              interval: weekly
          - package-ecosystem: github-actions
            directory: /
            schedule:
              interval: weekly

  via: push
YAML

export PS1='$ '
