#!/usr/bin/env bash
# Setup for RepositorySet plan demo: change one line in defaults, see diffs across all repos.
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

# Prepare mock data: 4 repos whose current state matches the OLD defaults
# Mock returns: required_reviews=1 (via branch protection), squash=true, delete_branch=true, wiki=false
# After editing defaults: required_reviews changes to 2 → all 4 repos show a diff
export MOCK_DIR=/tmp/mock-data

# The repositories are babarot's; GitHub has the description and topics the
# manifest sets.
description() {
  case "$1" in
    gh-infra) echo "Declarative GitHub infrastructure management via YAML" ;;
    httpstat) echo "It's like curl -v, with graphs and colors" ;;
    enhancd) echo ":rocket: A next-generation cd command with your interactive filter" ;;
    syno) echo "Find the Synology NAS on your network and check on it from the terminal" ;;
  esac
}
topics() {
  case "$1" in
    gh-infra) echo '[{"name":"cli"},{"name":"go"},{"name":"github"}]' ;;
    httpstat) echo '[{"name":"bash"},{"name":"httpstat"}]' ;;
    enhancd) echo '[{"name":"zsh"},{"name":"fzf"},{"name":"cd"}]' ;;
    syno) echo '[{"name":"cli"},{"name":"go"},{"name":"synology"}]' ;;
  esac
}

for repo in gh-infra httpstat enhancd syno; do
  mkdir -p "$MOCK_DIR/babarot/${repo}"
  cat > "$MOCK_DIR/babarot/${repo}/view.json" << JSON
{
  "description": "$(description "$repo")",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": $(topics "$repo"),
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": false,
  "hasWikiEnabled": false,
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": false,
  "squashMergeAllowed": true,
  "rebaseMergeAllowed": false,
  "deleteBranchOnMerge": true,
  "defaultBranchRef": { "name": "main" }
}
JSON
done

# Create the YAML that already has required_reviews: 1 (matching current state)
mkdir -p /tmp/demo

cat > /tmp/demo/repos.yaml << 'YAML'
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: babarot

defaults:
  spec:
    visibility: public
    features:
      wiki: false
    merge_strategy:
      allow_squash_merge: true
      auto_delete_head_branches: true
    branch_protection:
      - pattern: main
        required_reviews: 1

repositories:
  - name: gh-infra
    spec:
      description: "Declarative GitHub infrastructure management via YAML"
      topics: [cli, go, github]
  - name: httpstat
    spec:
      description: "It's like curl -v, with graphs and colors"
      topics: [bash, httpstat]
  - name: enhancd
    spec:
      description: ":rocket: A next-generation cd command with your interactive filter"
      topics: [zsh, fzf, cd]
  - name: syno
    spec:
      description: "Find the Synology NAS on your network and check on it from the terminal"
      topics: [cli, go, synology]
YAML

export PS1='$ '
