package infra

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"
	"unicode/utf8"
)

// maxCellRunes caps a table value so one long value cannot blow up a comment.
const maxCellRunes = 200

// WriteMarkdown renders the plan as a Markdown comment for a pull request:
// the summary, then one collapsed block per repository with a table of its
// changes, then warnings and errors.
func (d Document) WriteMarkdown(w io.Writer) error {
	var b strings.Builder

	if len(d.Changes) == 0 && len(d.Errors) == 0 {
		b.WriteString("### Plan: no changes\n")
	} else {
		fmt.Fprintf(&b, "### Plan: %d to create, %d to update, %d to destroy\n",
			d.Summary.Create, d.Summary.Update, d.Summary.Delete)
	}
	if len(d.Errors) > 0 {
		b.WriteString("\nThis plan is incomplete: some repositories could not be planned (see Errors).\n")
	}

	for _, target := range changeTargets(d.Changes) {
		writeTargetMarkdown(&b, target, d.Changes)
	}

	writeMessagesMarkdown(&b, "Warnings", d.Warnings)
	writeMessagesMarkdown(&b, "Errors", d.Errors)

	_, err := io.WriteString(w, b.String())
	return err
}

func changeTargets(changes []ChangeEntry) []string {
	var targets []string
	seen := make(map[string]bool)
	for _, c := range changes {
		if !seen[c.Target] {
			seen[c.Target] = true
			targets = append(targets, c.Target)
		}
	}
	return targets
}

func writeTargetMarkdown(b *strings.Builder, target string, changes []ChangeEntry) {
	var mine []ChangeEntry
	counts := map[string]int{}
	for _, c := range changes {
		if c.Target == target {
			mine = append(mine, c)
			counts[c.Action]++
		}
	}
	var parts []string
	for _, a := range []struct{ action, label string }{
		{"create", "to create"}, {"update", "to update"}, {"delete", "to destroy"},
	} {
		if n := counts[a.action]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, a.label))
		}
	}

	fmt.Fprintf(b, "\n<details><summary>%s: %s</summary>\n\n", code(target), strings.Join(parts, ", "))
	b.WriteString("| | Resource | Name | Change | Before | After |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, c := range mine {
		if c.Resource == fileResource {
			change := ""
			if c.Lines != nil {
				change = fmt.Sprintf("+%d -%d", c.Lines.Added, c.Lines.Removed)
			}
			if c.Mode != nil {
				change += fmt.Sprintf(", mode %s -> %s", modeOrNone(c.Mode.Before), c.Mode.After)
			}
			writeRow(b, c.Action, c.Resource, c.Path, strings.TrimPrefix(change, ", "), nil, nil, false)
			continue
		}
		if len(c.Details) == 0 {
			writeRow(b, c.Action, c.Resource, c.Name, c.Field, c.Before, c.After, true)
			continue
		}
		for _, row := range flattenDetails(c.Field, c.Details) {
			writeRow(b, row.Action, c.Resource, c.Name, row.Field, row.Before, row.After, true)
		}
	}

	for _, c := range mine {
		if c.Diff == "" {
			continue
		}
		fence := codeFence(c.Diff)
		fmt.Fprintf(b, "\n<details><summary>Diff of %s</summary>\n\n%sdiff\n%s", code(c.Path), fence, c.Diff)
		if !strings.HasSuffix(c.Diff, "\n") {
			b.WriteString("\n")
		}
		fmt.Fprintf(b, "%s\n\n</details>\n", fence)
	}

	b.WriteString("\n</details>\n")
}

func writeRow(b *strings.Builder, action, resource, name, change string, before, after any, codeChange bool) {
	changeCell := cell(change)
	if codeChange && change != "" {
		changeCell = code(change)
	}
	fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n",
		actionSymbol(action), cell(resource), codeOrEmpty(name), changeCell, valueCell(before), valueCell(after))
}

// flatDetail is one leaf of a grouped change, with its full field path.
type flatDetail struct {
	Field  string
	Action string
	Before any
	After  any
}

func flattenDetails(prefix string, details []Detail) []flatDetail {
	var out []flatDetail
	for _, d := range details {
		field := joinField(prefix, d.Field)
		if len(d.Details) > 0 {
			out = append(out, flattenDetails(field, d.Details)...)
			continue
		}
		out = append(out, flatDetail{Field: field, Action: d.Action, Before: d.Before, After: d.After})
	}
	return out
}

func joinField(prefix, field string) string {
	switch {
	case prefix == "":
		return field
	case field == "":
		return prefix
	}
	return prefix + "." + field
}

func writeMessagesMarkdown(b *strings.Builder, title string, msgs []Message) {
	if len(msgs) == 0 {
		return
	}
	fmt.Fprintf(b, "\n#### %s\n\n", title)
	for _, m := range msgs {
		if m.Target != "" {
			fmt.Fprintf(b, "- %s: %s\n", code(m.Target), cell(m.Message))
		} else {
			fmt.Fprintf(b, "- %s\n", cell(m.Message))
		}
	}
}

func actionSymbol(action string) string {
	switch action {
	case "create":
		return "+"
	case "delete":
		return "-"
	default:
		return "~"
	}
}

func modeOrNone(mode string) string {
	if mode == "" {
		return "(none)"
	}
	return mode
}

// valueCell renders a display value: strings as is, others as JSON.
func valueCell(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		raw, err := json.Marshal(v)
		if err != nil {
			s = fmt.Sprint(v)
		} else {
			s = string(raw)
		}
	}
	return code(s)
}

func codeOrEmpty(s string) string {
	if s == "" {
		return ""
	}
	return code(s)
}

// code wraps an escaped value in <code>, which unlike backticks needs no
// handling of backticks inside the value.
func code(s string) string {
	return "<code>" + cell(s) + "</code>"
}

// cell escapes s for a Markdown table cell or list item: HTML special
// characters, pipes, and newlines, and cuts it at maxCellRunes.
func cell(s string) string {
	if utf8.RuneCountInString(s) > maxCellRunes {
		s = string([]rune(s)[:maxCellRunes-1]) + "…"
	}
	s = html.EscapeString(s)
	s = strings.ReplaceAll(s, "|", "&#124;")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "<br>")
}

// codeFence returns a backtick fence longer than any backtick run in s, so
// the content cannot close the block.
func codeFence(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", max(3, longest+1))
}
