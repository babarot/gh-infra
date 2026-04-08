#!/usr/bin/env bash
# Setup for import --into demo with file changes (write/patch/skip cycle).
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
dir="$MOCK_DIR/babarot/my-project"
mkdir -p "$dir/contents/.github/workflows" "$dir/contents/.github"

# Repo view (matches local YAML — no repo setting diffs, only file diffs)
cat > "$dir/view.json" << 'JSON'
{
  "description": "My project",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [{"name":"go"},{"name":"cli"}],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": false,
  "hasWikiEnabled": false,
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": true,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": true,
  "defaultBranchRef": { "name": "main" }
}
JSON

# GitHub has DRIFTED versions of these files.
# import --into will detect the diffs and offer write/patch/skip.

# CODEOWNERS: someone added @team-temp on GitHub
cat > "$dir/contents/.github/CODEOWNERS" << 'OWNERS'
* @babarot @team-platform @team-temp
/docs/ @babarot
/api/ @babarot @team-backend
OWNERS

# ci.yml: someone added make bench step
cat > "$dir/contents/.github/workflows/ci.yml" << 'CI'
name: CI
on:
  push:
    branches: [main]
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: make test
      - run: make bench
CI

# dependabot.yml: someone changed interval to monthly
cat > "$dir/contents/.github/dependabot.yml" << 'DEPBOT'
version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: monthly
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: monthly
DEPBOT

# .golangci.yml: someone disabled errcheck
cat > "$dir/contents/.golangci.yml" << 'LINT'
linters:
  enable:
    - goimports
    - govet
    - staticcheck
    - unused
LINT

# PULL_REQUEST_TEMPLATE: someone rewrote the template
cat > "$dir/contents/.github/PULL_REQUEST_TEMPLATE.md" << 'TMPL'
## What changed

## Why

## How to test
TMPL

mkdir -p /tmp/demo

# Local manifest: the "intended" state. 5 files with inline content.
cat > /tmp/demo/my-project.yaml << 'YAML'
apiVersion: gh-infra/v1
kind: File
metadata:
  name: my-project
  owner: babarot

spec:
  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot @team-platform
        /docs/ @babarot
        /api/ @babarot @team-backend

    - path: .github/workflows/ci.yml
      content: |
        name: CI
        on:
          push:
            branches: [main]
          pull_request:
        jobs:
          test:
            runs-on: ubuntu-latest
            steps:
              - uses: actions/checkout@v4
              - uses: actions/setup-go@v5
                with:
                  go-version-file: go.mod
              - run: make test

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

    - path: .golangci.yml
      content: |
        linters:
          enable:
            - goimports
            - govet
            - errcheck
            - staticcheck
            - unused

    - path: .github/PULL_REQUEST_TEMPLATE.md
      content: |
        ## What
        <!-- Describe the change -->

        ## Why
        <!-- Why is this needed? -->

        ## Testing
        - [ ] Unit tests
        - [ ] Manual verification

  via: push
YAML

export PS1='$ '
