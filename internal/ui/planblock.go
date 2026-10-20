package ui

import (
	"bytes"
	"fmt"
	"slices"
)

// PlanBlock is one block of the plan body: a repository's header and the
// changes planned for it. The plan body is built as blocks, rewritten by
// RewritePlanBlocks, and printed by RenderPlanBlocks.
type PlanBlock struct {
	Names  []string     // repositories, in plan order; several after merging
	Icon   string       // IconChange, IconAdd (created) or IconRemove (destroyed)
	Right  string       // header's right-hand text for one repository, e.g. "2 to update"
	Groups []DiffGroup  // settings changes
	Notes  []string     // notes after the settings, e.g. what a create leaves to the next plan
	Files  *FileBlock   // FileSet changes; nil without any
	Unseen Unseen       // what apply does that the body does not show, without the name
	Differ []UnseenPart // after merging: the parts of Unseen that differ among Names
}

// FileBlock holds the FileSet changes of a PlanBlock.
type FileBlock struct {
	Via   string // delivery method, "" when unknown
	Items []FileItem
	Diffs []string // per item; "" without --diff or without a diff
	Notes []string // notes after the files, e.g. why a pull_request FileSet is pushed
}

// Unseen holds, per part, what apply does to a repository beyond what the
// plan body shows. The caller fills it; it is only compared, never printed,
// since it can hold secret values.
type Unseen struct {
	Settings string // spec values apply sends that the plan summarizes or leaves out
	Files    string // file contents and modes
	Delivery string // how the files are delivered: branch, commit message, pull request
}

// UnseenPart names a part of Unseen.
type UnseenPart int

const (
	UnseenSettings UnseenPart = iota
	UnseenFiles
	UnseenDelivery
)

var unseenParts = []UnseenPart{UnseenSettings, UnseenFiles, UnseenDelivery}

func (u Unseen) part(p UnseenPart) string {
	switch p {
	case UnseenSettings:
		return u.Settings
	case UnseenFiles:
		return u.Files
	default:
		return u.Delivery
	}
}

// note is how the header of a merged block says the part differs.
func (p UnseenPart) note() string {
	switch p {
	case UnseenSettings:
		return "settings the plan shows only in summary"
	case UnseenFiles:
		return "file contents"
	default:
		return "how the files are delivered (pull request branch, commit message)"
	}
}

// PlanPass rewrites the blocks of a plan body. A pass knows nothing of the
// terminal, so it is tested on blocks alone.
type PlanPass func([]PlanBlock) []PlanBlock

// planPasses are the passes the plan body goes through, in order.
var planPasses = []PlanPass{flattenBlocks, mergeIdentical}

// flattenBlocks runs FlattenSingleChild on the settings of each block.
func flattenBlocks(blocks []PlanBlock) []PlanBlock {
	out := make([]PlanBlock, len(blocks))
	for i, b := range blocks {
		b.Groups = FlattenSingleChild(b.Groups)
		out[i] = b
	}
	return out
}

// mergeIdentical prints the repositories whose blocks read the same once:
// it joins the names of each IconChange block whose body and Right match an
// earlier one into that block, which keeps its place. A body is compared as
// printed without a terminal, so no value is cut short, and the merged block
// shows each repository exactly what it would show alone. What apply does
// that the body does not show is not compared; the parts of Unseen that
// differ go to Differ, for the header to say.
func mergeIdentical(blocks []PlanBlock) []PlanBlock {
	var out []PlanBlock
	var unseen [][]Unseen // per block of out, its repositories' Unseen
	first := make(map[string]int)
	for _, b := range blocks {
		if b.Icon == IconChange {
			key := b.Right + "\n" + blockBody(b)
			if i, ok := first[key]; ok {
				out[i].Names = append(out[i].Names, b.Names...)
				unseen[i] = append(unseen[i], b.Unseen)
				continue
			}
			first[key] = len(out)
		}
		b.Names = slices.Clone(b.Names)
		out = append(out, b)
		unseen = append(unseen, []Unseen{b.Unseen})
	}
	for i := range out {
		out[i].Differ = nil
		for _, part := range unseenParts {
			for _, u := range unseen[i][1:] {
				if u.part(part) != unseen[i][0].part(part) {
					out[i].Differ = append(out[i].Differ, part)
					break
				}
			}
		}
	}
	return out
}

// blockBody is the body of b as printed without a terminal.
func blockBody(b PlanBlock) string {
	var buf bytes.Buffer
	renderBlockBody(NewStandardPrinterWith(&buf, &buf), b)
	return buf.String()
}

// RewritePlanBlocks runs blocks through the plan body's passes.
func RewritePlanBlocks(blocks []PlanBlock) []PlanBlock {
	for _, pass := range planPasses {
		blocks = pass(blocks)
	}
	return blocks
}

// RenderPlanBlocks prints the plan body.
func RenderPlanBlocks(p Printer, blocks []PlanBlock) {
	for _, b := range blocks {
		if len(b.Names) > 1 {
			notes := make([]string, len(b.Differ))
			for i, part := range b.Differ {
				notes[i] = part.note()
			}
			p.RepoGroupHeader(b.Icon, b.Names, b.Right+" each", notes)
		} else {
			p.RepoHeader(b.Icon, b.Names[0], "", b.Right)
		}
		renderBlockBody(p, b)
		p.GroupEnd()
		p.SetColumnWidth(0)
	}
}

// renderBlockBody prints what follows the header of b.
func renderBlockBody(p Printer, b PlanBlock) {
	p.SetColumnWidth(DiffGroupFieldWidth(b.Groups))
	RenderDiffGroups(p, b.Groups)
	for _, n := range b.Notes {
		p.Detail(n)
	}
	if f := b.Files; f != nil {
		renderFileBlock(p, f)
	}
}

func renderFileBlock(p Printer, f *FileBlock) {
	w := 0
	for _, item := range f.Items {
		w = max(w, len(item.Path))
	}
	p.SetColumnWidth(w)
	label := fmt.Sprintf("%d file", len(f.Items))
	if len(f.Items) != 1 {
		label += "s"
	}
	if f.Via != "" {
		label += ", via " + Cyan.Render(f.Via)
	}
	p.SubGroupHeader(IconChange, "FileSet: "+Bold.Render(label))
	for i, item := range f.Items {
		p.PrintFileChange(item)
		if f.Diffs[i] != "" {
			p.PrintDiffBlock(f.Diffs[i])
		}
	}
	for _, n := range f.Notes {
		p.Detail(n)
	}
}
