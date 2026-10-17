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

for repo in afx gomi; do
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

  # Old CODEOWNERS (one owner for everything → gets per-path entries)
  cat > "$dir/contents/.github/CODEOWNERS" << 'OWNERS'
* @babarot
OWNERS

  # Old CI workflow (checkout v3, push only, no lint)
  cat > "$dir/contents/.github/workflows/ci.yml" << 'CI'
name: CI
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version-file: go.mod
      - run: make test
CI

  # dependabot.yml does NOT exist on GitHub → new file
done

mkdir -p /tmp/demo

cat > /tmp/demo/files.yaml << 'YAML'
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: babarot

spec:
  repositories:
    - afx
    - gomi

  files:
    - path: .github/CODEOWNERS
      content: |
        * @babarot
        /docs/ @babarot
        /.github/ @babarot

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
              - run: make lint
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

  via: push
YAML

export PS1='$ '
