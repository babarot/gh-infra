package fileset

import (
	"fmt"
	"slices"
	"strings"

	"github.com/babarot/gh-infra/internal/manifest"
)

// FileContext is one manifest file entry in .Files of commit_message,
// pr_title and pr_body: the files of the entry that the commit changes in
// the target repository.
type FileContext struct {
	Path        string   // the entry's path; the directory for a directory source
	Paths       []string // the changed files of the entry, in plan order
	Count       int      // len(Paths)
	Type        string   // create, update or delete, or mixed
	Action      string   // Add, Update or Remove; Update for mixed
	Description string   // the entry's description, rendered, without trailing newlines
	dir         bool
}

// messageContext is the data available to commit_message, pr_title and
// pr_body. Vars are not available; use .Repo, .Source and .Files.
type messageContext struct {
	Repo   RepoContext
	Source SourceContext
	Files  []FileContext
}

// buildFiles groups changes by the manifest entry that planned them, in
// manifest order, leaving out changes with nothing to do. Descriptions are
// rendered against repo and sourceURL.
func buildFiles(changes []Change, repo, sourceURL string) ([]FileContext, error) {
	var files []FileContext
	order := make(map[string]int) // entry -> EntryIndex
	index := make(map[string]int) // entry -> position in files
	for _, c := range changes {
		if c.Type == ChangeNoOp {
			continue
		}
		entry := c.Entry
		if entry == "" {
			entry = c.Path
		}
		i, ok := index[entry]
		if !ok {
			desc, err := renderDescription(c.Description, repo, sourceURL)
			if err != nil {
				return nil, fmt.Errorf("description of %s: %w", entry, err)
			}
			i = len(files)
			index[entry] = i
			order[entry] = c.EntryIndex
			files = append(files, FileContext{Path: entry, Type: string(c.Type), Description: desc})
		}
		f := &files[i]
		f.Paths = append(f.Paths, c.Path)
		f.Count++
		if f.Type != string(c.Type) {
			f.Type = "mixed"
		}
		if c.Path != entry {
			f.dir = true
		}
	}
	slices.SortStableFunc(files, func(a, b FileContext) int { return order[a.Path] - order[b.Path] })
	for i := range files {
		files[i].Action = actionOf(files[i].Type)
	}
	return files, nil
}

// entryFiles is .Files for checking message templates when there are no
// changes to build it from: every entry of files, as if it were created.
func entryFiles(files []manifest.FileEntry, repo string) ([]FileContext, error) {
	var out []FileContext
	index := make(map[string]int)
	for _, f := range files {
		entry := f.Path
		if f.DirScope != "" {
			entry = f.DirScope
		}
		i, ok := index[entry]
		if !ok {
			desc, err := renderDescription(f.Description, repo, "")
			if err != nil {
				return nil, fmt.Errorf("description of %s: %w", entry, err)
			}
			i = len(out)
			index[entry] = i
			out = append(out, FileContext{
				Path:        entry,
				Type:        string(ChangeCreate),
				Action:      actionOf(string(ChangeCreate)),
				Description: desc,
				dir:         f.DirScope != "",
			})
		}
		out[i].Paths = append(out[i].Paths, f.Path)
		out[i].Count++
	}
	return out, nil
}

func renderDescription(desc, repo, sourceURL string) (string, error) {
	desc = strings.TrimRight(desc, "\n")
	if !HasTemplate(desc, nil) {
		return desc, nil
	}
	return renderMessage(desc, repo, sourceURL, nil)
}

// actionOf is the verb for a FileContext Type in the default messages.
func actionOf(typ string) string {
	switch ChangeType(typ) {
	case ChangeCreate:
		return "Add"
	case ChangeDelete:
		return "Remove"
	default:
		return "Update"
	}
}

// defaultCommitMessage is the commit message without commit_message: the
// change of a single file, or the number of files changed.
func defaultCommitMessage(files []FileContext) string {
	total := 0
	typ := ""
	for i, f := range files {
		total += f.Count
		if i == 0 {
			typ = f.Type
		} else if typ != f.Type {
			typ = "mixed"
		}
	}
	switch total {
	case 0:
		return "Update files"
	case 1:
		return files[0].Action + " " + files[0].Paths[0]
	default:
		return fmt.Sprintf("%s %d files", actionOf(typ), total)
	}
}

// defaultPRBody is the pull request body without pr_body: a line per entry
// with its action, path and description.
func defaultPRBody(files []FileContext) string {
	var b strings.Builder
	for _, f := range files {
		path := "`" + f.Path + "`"
		if f.dir {
			path = "`" + strings.TrimSuffix(f.Path, "/") + "/`"
			noun := "files"
			if f.Count == 1 {
				noun = "file"
			}
			path += fmt.Sprintf(" (%d %s)", f.Count, noun)
		}
		fmt.Fprintf(&b, "- %s %s", f.Action, path)
		if f.Description != "" {
			// Continuation lines are indented to stay in the list item.
			fmt.Fprintf(&b, ": %s", strings.ReplaceAll(f.Description, "\n", "\n  "))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
