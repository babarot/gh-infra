#!/usr/bin/env bash
# Setup for apply demo: RepositorySet with 3 repos showing parallel spinners.
# The repositories are babarot's; their descriptions on GitHub match the
# manifest, the rest is stale.
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

description() {
  case "$1" in
    gomi) echo "🗑️ Your UNIX rm command with a safety net!" ;;
    afx) echo "An alternative, fast package manager for plugins and executable commands" ;;
    gist) echo ":octocat: A simple gist editor for CLI" ;;
  esac
}

for repo in gomi afx gist; do
  mkdir -p "$MOCK_DIR/babarot/${repo}"
done

for repo in gomi afx gist; do
  cat > "$MOCK_DIR/babarot/${repo}/view.json" << JSON
{
  "description": "$(description "$repo")",
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
  - name: gomi
    spec:
      description: "🗑️ Your UNIX rm command with a safety net!"
      topics: [trash, go, cli, rm]
  - name: afx
    spec:
      description: "An alternative, fast package manager for plugins and executable commands"
      topics: [cli, package-manager, go]
  - name: gist
    spec:
      description: ":octocat: A simple gist editor for CLI"
      topics: [gist, go, cli]
YAML

export PS1='$ '
