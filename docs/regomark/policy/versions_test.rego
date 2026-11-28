package docs_test

import rego.v1

import data.docs

changelog := {"path": "CHANGELOG.md", "blocks": [
	{"kind": "heading", "level": 2, "line": 3, "text": "v0.16.0 - 2026-10-12"},
	{"kind": "heading", "level": 2, "line": 9, "text": "v0.15.1 - 2026-10-10"},
	{"kind": "heading", "level": 3, "line": 10, "text": "Bug fixes v0.99.0"},
]}

page(text) := {"path": "docs/src/content/docs/a.md", "blocks": [{"kind": "paragraph", "line": 5, "text": text}]}

rules(text) := {d.rule | some d in docs.deny with input as {"documents": [changelog, page(text)]}}

test_a_note_naming_a_release_it_keeps_passes if {
	count(rules(":::caution[Changed in v0.15.1]\nBefore v0.15.1, it inherited.\n:::")) == 0
}

test_a_note_with_no_release_fails if {
	rules(":::caution[Changed recently]\nIt inherited.\n:::") == {"note-version"}
}

test_a_note_whose_text_names_another_release_fails if {
	rules(":::caution[Changed in v0.15.1]\nBefore v0.16.0, it inherited.\n:::") == {"note-version-mismatch"}
}

test_a_note_naming_a_release_not_in_the_changelog_fails if {
	rules(":::note[Added in v0.99.0]\nSomething.\n:::") == {"note-version-unknown"}
}

test_other_asides_are_left_alone if {
	count(rules(":::note[Before you start]\nSee v0.1.0.\n:::")) == 0
	count(rules(":::tip\nUse v0.2.0.\n:::")) == 0
}
