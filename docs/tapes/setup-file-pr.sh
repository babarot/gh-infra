#!/usr/bin/env bash
# Setup for File PR demo: distribute many files to a repo via pull request.
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
mkdir -p "$dir/contents/.github/workflows"

# Repo view (no repo settings changes — just needed for plan)
cat > "$dir/view.json" << 'JSON'
{
  "description": "My project",
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

# Existing files on GitHub (old versions — will show as updates)
echo -n '* @old-owner' > "$dir/contents/.github/CODEOWNERS"

cat > "$dir/contents/.github/workflows/ci.yml" << 'CI'
name: CI
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - run: make test
CI

printf 'MIT License\n\nCopyright (c) 2024' > "$dir/contents/LICENSE"

# These files do NOT exist on GitHub (will show as creates):
#   .github/workflows/release.yml
#   .github/dependabot.yml
#   .github/PULL_REQUEST_TEMPLATE.md
#   .golangci.yml

mkdir -p /tmp/demo

cat > /tmp/demo/files.yaml << 'YAML'
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

    - path: LICENSE
      content: |
        MIT License

        Copyright (c) 2026 babarot

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
              - uses: golangci/golangci-lint-action@v6
          test:
            runs-on: ubuntu-latest
            steps:
              - uses: actions/checkout@v4
              - run: make test
              - uses: codecov/codecov-action@v4

    - path: .github/workflows/release.yml
      content: |
        name: Release
        on:
          push:
            tags: ["v*"]
        jobs:
          release:
            runs-on: ubuntu-latest
            steps:
              - uses: actions/checkout@v4
              - uses: goreleaser/goreleaser-action@v6
                with:
                  args: release --clean

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

    - path: .github/PULL_REQUEST_TEMPLATE.md
      content: |
        ## What
        <!-- What does this PR do? -->

        ## Why
        <!-- Why is this change needed? -->

        ## Testing
        - [ ] Unit tests
        - [ ] Manual verification

    - path: .golangci.yml
      content: |
        linters:
          enable:
            - goimports
            - govet
            - errcheck
            - staticcheck
            - unused

  via: pull_request
  commit_message: "chore: sync managed files"
  branch: gh-infra/sync
  pr_title: "chore: sync managed files"
  pr_body: "Automated update by gh-infra"
YAML

export PS1='$ '
