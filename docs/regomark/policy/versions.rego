# METADATA
# title: Release notes in the docs
# description: >-
#   An aside whose title says when something changed names the release, the
#   same release throughout, and one that CHANGELOG.md lists. Whether it is
#   the right release cannot be checked here. The release fields and flags
#   arrived in is marked on its own (docs/src/plugins/since.mjs), so these
#   notes are for changes in behavior.
# entrypoint: true
package docs

import rego.v1

schema_version := 3

rule_ids := {"note-version", "note-version-mismatch", "note-version-unknown"}

version_re := `v\d+\.\d+\.\d+`

# The releases: the versions in the level-2 headings of CHANGELOG.md
released contains v if {
	some doc in input.documents
	doc.path == "CHANGELOG.md"
	some b in doc.blocks
	b.kind == "heading"
	b.level == 2
	some v in regex.find_n(version_re, b.text, 1)
}

# Asides titled with when something changed: :::caution[Changed in v0.15.1]
notes contains {"path": doc.path, "line": b.line, "title": title, "body": body} if {
	some doc in input.documents
	doc.path != "CHANGELOG.md"
	some b in doc.blocks
	b.kind == "paragraph"
	some m in regex.find_all_string_submatch_n(`^:::[a-z]+\[([^\]]*)\]`, b.text, 1)
	title := m[1]
	regex.match(`(?i)^(added|changed|deprecated|removed)\b`, title)
	body := substring(b.text, indexof(b.text, "\n") + 1, -1)
}

deny contains {"rule": "note-version", "path": n.path, "line": n.line, "message": msg} if {
	some n in notes
	not regex.match(version_re, n.title)
	msg := sprintf("%q names no release; write it as vX.Y.Z", [n.title])
}

deny contains {"rule": "note-version-mismatch", "path": n.path, "line": n.line, "message": msg} if {
	some n in notes
	[tv] := regex.find_n(version_re, n.title, 1)
	some bv in regex.find_n(version_re, n.body, -1)
	bv != tv
	msg := sprintf("the title says %s, the text %s", [tv, bv])
}

deny contains {"rule": "note-version-unknown", "path": n.path, "line": n.line, "message": msg} if {
	some n in notes
	some v in regex.find_n(version_re, n.title, -1)
	not v in released
	msg := sprintf("%s is not a release in CHANGELOG.md; name the release that made the change", [v])
}
