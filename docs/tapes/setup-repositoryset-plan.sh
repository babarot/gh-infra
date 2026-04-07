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

for repo in my-cli my-api my-web my-bot; do
  mkdir -p "$MOCK_DIR/babarot/${repo}"
  cat > "$MOCK_DIR/babarot/${repo}/view.json" << JSON
{
  "description": "$(echo "$repo" | sed 's/-/ /g' | sed 's/my //')",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [],
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
  - name: my-cli
    spec:
      description: "CLI tool"
      topics: [go, cli]
  - name: my-api
    spec:
      description: "Backend API"
      topics: [go, api]
  - name: my-web
    spec:
      description: "Web frontend"
      topics: [typescript, react]
  - name: my-bot
    spec:
      description: "Slack bot"
      topics: [go, slack]
YAML

export PS1='$ '
