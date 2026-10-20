package ui

import "fmt"

// PlanBlock is one block of the plan body: a repository's header and the
// changes planned for it. The plan body is built as blocks, rewritten by
// RewritePlanBlocks, and printed by RenderPlanBlocks.
type PlanBlock struct {
	Names  []string    // repositories, in plan order
	Icon   string      // IconChange, IconAdd (created) or IconRemove (destroyed)
	Right  string      // header's right-hand text for one repository, e.g. "2 to update"
	Groups []DiffGroup // settings changes
	Notes  []string    // notes after the settings, e.g. what a create leaves to the next plan
	Files  *FileBlock  // FileSet changes; nil without any
}

// FileBlock holds the FileSet changes of a PlanBlock.
type FileBlock struct {
	Via   string // delivery method, "" when unknown
	Items []FileItem
	Diffs []string // per item; "" without --diff or without a diff
	Notes []string // notes after the files, e.g. why a pull_request FileSet is pushed
}

// PlanPass rewrites the blocks of a plan body. A pass knows nothing of the
// printer or the terminal, so it is tested on blocks alone.
type PlanPass func([]PlanBlock) []PlanBlock

// planPasses are the passes the plan body goes through, in order.
var planPasses []PlanPass

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
		p.RepoHeader(b.Icon, b.Names[0], "", b.Right)
		p.SetColumnWidth(DiffGroupFieldWidth(b.Groups))
		RenderDiffGroups(p, b.Groups)
		for _, n := range b.Notes {
			p.Detail(n)
		}
		if f := b.Files; f != nil {
			renderFileBlock(p, f)
		}
		p.GroupEnd()
		p.SetColumnWidth(0)
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
