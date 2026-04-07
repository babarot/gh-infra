#!/usr/bin/env bash
# Setup for apply demo: RepositorySet with 3 repos showing parallel spinners.
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

# Prepare mock data: 3 repos with stale settings
# Mock returns: squash=false, delete_branch=false, wiki=true, topics=[]
# YAML wants:   squash=true,  delete_branch=true,  wiki=false, topics=[...]
export MOCK_DIR=/tmp/mock-data

for repo in app-api app-web app-cli; do
  mkdir -p "$MOCK_DIR/babarot/${repo}"
done

for repo in app-api app-web app-cli; do
  cat > "$MOCK_DIR/babarot/${repo}/view.json" << JSON
{
  "description": "$(echo "$repo" | sed 's/-/ /g')",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": true,
  "hasWikiEnabled": true,
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": false,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": false,
  "defaultBranchRef": { "name": "main" }
}
JSON
done

mkdir -p /tmp/demo

cat > /tmp/demo/repos.yaml << 'YAML'
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: babarot

defaults:
  spec:
    features:
      wiki: false
    merge_strategy:
      allow_squash_merge: true
      auto_delete_head_branches: true

repositories:
  - name: app-api
    spec:
      description: "Backend API service"
      topics: [go, api, grpc]
  - name: app-web
    spec:
      description: "Web frontend"
      topics: [typescript, react]
  - name: app-cli
    spec:
      description: "CLI tool"
      topics: [go, cli]
YAML

export PS1='$ '
