#!/usr/bin/env bash
# Try gh-infra's terminal output locally against mock-gh, without GitHub.
#
# Builds gh-infra from this working tree, writes a scenario (manifests and
# the mock's "GitHub state") to a temporary directory, and opens a shell in
# which `gh` is mock-gh. Run `gh infra plan repos/` or `gh infra apply repos/`
# there; apply changes nothing (mock-gh only answers), so plan shows the same
# changes again afterwards. Exiting the shell removes the directory.
#
# Usage: playground.sh [mixed|many]
#   mixed  five repositories: changes, a FileSet via push and via pull
#          request, one up to date, one that cannot be read (403), one that
#          does not exist yet (created by apply), and a write that fails (422)
#   many   40 repositories, mostly up to date, for the output of large sets
set -euo pipefail

scenario="${1:-mixed}"
case "$scenario" in
  mixed | many) ;;
  *) echo "usage: $0 [mixed|many]" >&2; exit 2 ;;
esac

root="$(cd "$(dirname "$0")/../.." && pwd)"
mock_gh="$root/docs/tapes/mock-gh"
work="$(mktemp -d "${TMPDIR:-/tmp}/gh-infra-playground.XXXXXX")"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/mock" "$work/repos"

echo "Building gh-infra from $root ..."
(cd "$root" && go build -o "$work/bin/gh-infra" ./cmd/gh-infra)

# Every gh call, from gh-infra or typed in the shell, goes to mock-gh.
cat > "$work/bin/gh" << EOF
#!/usr/bin/env bash
if [[ "\${1:-}" == "infra" ]]; then
  shift
  exec "$work/bin/gh-infra" "\$@"
fi
exec "$mock_gh" "\$@"
EOF
chmod +x "$work/bin/gh"

# view_json <description> <has_wiki> <squash> <delete_branch>
view_json() {
  cat << EOF
{
  "description": "$1",
  "homepageUrl": "",
  "visibility": "PUBLIC",
  "isArchived": false,
  "repositoryTopics": [],
  "hasIssuesEnabled": true,
  "hasProjectsEnabled": true,
  "hasWikiEnabled": $2,
  "hasDiscussionsEnabled": false,
  "mergeCommitAllowed": true,
  "squashMergeAllowed": $3,
  "rebaseMergeAllowed": true,
  "deleteBranchOnMerge": $4,
  "defaultBranchRef": { "name": "main" }
}
EOF
}

# repo <name>: the mock's directory for acme/<name>
repo() {
  mkdir -p "$work/mock/acme/$1"
  echo "$work/mock/acme/$1"
}

license='MIT License

Copyright (c) acme
'
ci='name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make test
'

write_mixed() {
  local d

  # api: settings and topics to change; one file to update, one to create
  d="$(repo api)"
  view_json "api server" true false false > "$d/view.json"
  mkdir -p "$d/contents"
  printf 'MIT License\n\nCopyright (c) 2023\n' > "$d/contents/LICENSE"

  # web: settings to change, and its description update fails at apply
  d="$(repo web)"
  view_json "web" true false false > "$d/view.json"
  echo "write --description HTTP 422: Validation Failed: description is invalid" > "$d/fail"

  # cli: already as declared, files included
  d="$(repo cli)"
  view_json "Command line tool" false true true > "$d/view.json"
  mkdir -p "$d/contents/.github/workflows"
  printf '%s' "$license" > "$d/contents/LICENSE"
  printf '%s' "$ci" > "$d/contents/.github/workflows/ci.yaml"

  # docs: the token cannot read its settings
  d="$(repo docs)"
  echo "read description,homepageUrl HTTP 403: Resource not accessible by integration" > "$d/fail"

  # new-app: does not exist yet
  d="$(repo new-app)"
  touch "$d/absent"

  cat > "$work/repos/repositories.yaml" << 'EOF'
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: acme

defaults:
  spec:
    features:
      wiki: false
    merge_strategy:
      allow_squash_merge: true
      auto_delete_head_branches: true

repositories:
  - name: api
    spec:
      description: "API server for acme, with a description long enough to wrap in a narrow terminal"
      topics: [go, api, grpc]
  - name: web
    spec:
      description: "Web frontend"
      topics: [typescript, react]
  - name: cli
    spec:
      description: "Command line tool"
  - name: docs
    spec:
      description: "Documentation site"
  - name: new-app
    spec:
      description: "A new application"
EOF

  cat > "$work/repos/shared-files.yaml" << EOF
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: acme
  name: shared-files
spec:
  repositories:
    - api
    - cli
  files:
    - path: LICENSE
      content: |
$(printf '%s' "$license" | sed 's/^/        /')
    - path: .github/workflows/ci.yaml
      content: |
$(printf '%s' "$ci" | sed 's/^/        /')
  via: push
  commit_message: "chore: sync shared files"
EOF

  cat > "$work/repos/web-files.yaml" << 'EOF'
apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: acme
  name: web-files
spec:
  repositories:
    - web
  files:
    - path: .github/CODEOWNERS
      content: |
        * @acme/web
  via: pull_request
EOF
}

write_many() {
  local i name d
  {
    cat << 'EOF'
apiVersion: gh-infra/v1
kind: RepositorySet
metadata:
  owner: acme

defaults:
  spec:
    features:
      wiki: false
    merge_strategy:
      allow_squash_merge: true
      auto_delete_head_branches: true

repositories:
EOF
    for i in $(seq -w 1 40); do
      printf '  - name: repo-%s\n    spec:\n      description: "Repository %s"\n' "$i" "$i"
    done
  } > "$work/repos/repositories.yaml"

  for i in $(seq -w 1 40); do
    name="repo-$i"
    d="$(repo "$name")"
    case "$i" in
      07 | 19 | 33) view_json "Repository $i" true false true > "$d/view.json" ;; # wiki and squash to change
      26) echo "read description,homepageUrl HTTP 403: Resource not accessible by integration" > "$d/fail" ;;
      38) touch "$d/absent" ;;
      *) view_json "Repository $i" false true true > "$d/view.json" ;;
    esac
  done
}

"write_$scenario"

# Refuse to start unless `gh` resolves to the mock in the shell's PATH.
resolved="$(PATH="$work/bin:$PATH" command -v gh)"
if [[ "$resolved" != "$work/bin/gh" ]]; then
  echo "gh resolves to $resolved, not the mock; refusing to start" >&2
  exit 1
fi

cat > "$work/rc" << EOF
export PATH="$work/bin:\$PATH"
export MOCK_DIR="$work/mock"
PS1='(gh-infra playground: $scenario) \$ '
cd "$work"
if [[ "\$(command -v gh)" != "$work/bin/gh" ]]; then
  echo "gh does not resolve to the mock; exiting" >&2
  exit 1
fi
EOF

cat << EOF

gh-infra playground ($scenario), in $work
gh here is mock-gh: nothing reaches GitHub, and apply changes nothing.

  gh infra plan repos/
  gh infra apply repos/
  GH_INFRA_OUTPUT=stream gh infra apply repos/

Mock state is in mock/ (see the comments at the top of $mock_gh).
Type exit to leave; the directory is then removed.

EOF
BASH_SILENCE_DEPRECATION_WARNING=1 bash --noprofile --rcfile "$work/rc" -i || true
