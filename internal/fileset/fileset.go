package fileset

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/output"
)

// FileState represents the current state of a file in a repository.
type FileState struct {
	Path    string
	Content string
	SHA     string // needed for updates via Contents API
	Exists  bool
}

// FileChange represents a planned change for a file.
type FileChange struct {
	FileSet string // FileSet name
	Target  string // owner/repo
	Path    string
	Type    ChangeType
	Current string // current content (if exists)
	Desired string // desired content
	SHA     string // current SHA (for updates)
	OnDrift string // warn, overwrite, skip
	Drifted bool   // file exists but content differs
}

type ChangeType string

const (
	FileCreate ChangeType = "create"
	FileUpdate ChangeType = "update"
	FileNoOp   ChangeType = "noop"
	FileDrift  ChangeType = "drift"
	FileSkip   ChangeType = "skip"
)

// Processor handles FileSet plan and apply operations.
type Processor struct {
	runner gh.Runner
}

func NewProcessor(runner gh.Runner) *Processor {
	return &Processor{runner: runner}
}

// Plan computes changes for all FileSets.
func (p *Processor) Plan(fileSets []*manifest.FileSet) []FileChange {
	var changes []FileChange

	for _, fs := range fileSets {
		for _, target := range fs.Spec.Targets {
			files := manifest.ResolveFiles(fs, target)
			for _, file := range files {
				change := p.planFile(fs.Metadata.Name, target.Name, file, fs.Spec.OnDrift)
				changes = append(changes, change)
			}
		}
	}

	return changes
}

func (p *Processor) planFile(fileSetName, repo string, file manifest.FileEntry, onDrift string) FileChange {
	current, err := p.fetchFileContent(repo, file.Path)
	if err != nil || !current.Exists {
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileCreate,
			Desired: file.Content,
			OnDrift: onDrift,
		}
	}

	// Normalize for comparison (trim trailing newlines)
	currentContent := strings.TrimRight(current.Content, "\n")
	desiredContent := strings.TrimRight(file.Content, "\n")

	if currentContent == desiredContent {
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileNoOp,
			OnDrift: onDrift,
		}
	}

	// Content differs — drift detected
	switch onDrift {
	case manifest.OnDriftSkip:
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileSkip,
			Current: current.Content,
			Desired: file.Content,
			SHA:     current.SHA,
			OnDrift: onDrift,
			Drifted: true,
		}
	case manifest.OnDriftWarn:
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileDrift,
			Current: current.Content,
			Desired: file.Content,
			SHA:     current.SHA,
			OnDrift: onDrift,
			Drifted: true,
		}
	default: // "overwrite"
		return FileChange{
			FileSet: fileSetName,
			Target:  repo,
			Path:    file.Path,
			Type:    FileUpdate,
			Current: current.Content,
			Desired: file.Content,
			SHA:     current.SHA,
			OnDrift: onDrift,
			Drifted: true,
		}
	}
}

func (p *Processor) fetchFileContent(repo, path string) (*FileState, error) {
	out, err := p.runner.Run("api", fmt.Sprintf("repos/%s/contents/%s", repo, path))
	if err != nil {
		return &FileState{Path: path, Exists: false}, err
	}

	var raw struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return &FileState{Path: path, Exists: false}, err
	}

	content := raw.Content
	if raw.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("decode base64 for %s: %w", path, err)
		}
		content = string(decoded)
	}

	return &FileState{
		Path:    path,
		Content: content,
		SHA:     raw.SHA,
		Exists:  true,
	}, nil
}

// Apply executes the planned file changes.
func (p *Processor) Apply(changes []FileChange) []FileApplyResult {
	var results []FileApplyResult
	for _, c := range changes {
		switch c.Type {
		case FileCreate:
			err := p.createFile(c.Target, c.Path, c.Desired)
			results = append(results, FileApplyResult{Change: c, Err: err})
		case FileUpdate:
			err := p.updateFile(c.Target, c.Path, c.Desired, c.SHA)
			results = append(results, FileApplyResult{Change: c, Err: err})
		case FileDrift:
			// warn mode: skip apply but report
			results = append(results, FileApplyResult{Change: c, Skipped: true})
		case FileSkip, FileNoOp:
			// do nothing
		}
	}
	return results
}

type FileApplyResult struct {
	Change  FileChange
	Err     error
	Skipped bool
}

func (p *Processor) createFile(repo, path, content string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	endpoint := fmt.Sprintf("repos/%s/contents/%s", repo, path)
	_, err := p.runner.Run("api", endpoint,
		"--method", "PUT",
		"-f", fmt.Sprintf("message=chore: add %s via gh-infra", path),
		"-f", fmt.Sprintf("content=%s", encoded),
	)
	return err
}

func (p *Processor) updateFile(repo, path, content, sha string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	endpoint := fmt.Sprintf("repos/%s/contents/%s", repo, path)
	_, err := p.runner.Run("api", endpoint,
		"--method", "PUT",
		"-f", fmt.Sprintf("message=chore: update %s via gh-infra", path),
		"-f", fmt.Sprintf("content=%s", encoded),
		"-f", fmt.Sprintf("sha=%s", sha),
	)
	return err
}

// PrintPlan prints FileSet changes to the writer.
func PrintPlan(w io.Writer, changes []FileChange) {
	if len(changes) == 0 {
		return
	}

	hasChanges := false
	for _, c := range changes {
		if c.Type != FileNoOp {
			hasChanges = true
			break
		}
	}
	if !hasChanges {
		return
	}

	// Group by fileset+target
	type groupKey struct{ fileSet, target string }
	type group struct {
		key     groupKey
		changes []FileChange
	}
	seen := make(map[groupKey]int)
	var groups []group

	for _, c := range changes {
		if c.Type == FileNoOp {
			continue
		}
		k := groupKey{c.FileSet, c.Target}
		idx, ok := seen[k]
		if !ok {
			idx = len(groups)
			seen[k] = idx
			groups = append(groups, group{key: k})
		}
		groups[idx].changes = append(groups[idx].changes, c)
	}

	for _, g := range groups {
		fmt.Fprintf(w, "  %s FileSet: %s → %s\n",
			output.Yellow.Render("~"),
			output.Bold.Render(g.key.fileSet),
			output.Bold.Render(g.key.target))
		for _, c := range g.changes {
			switch c.Type {
			case FileCreate:
				fmt.Fprintf(w, "      %s %s  %s\n",
					output.Green.Render("+"), c.Path, output.Green.Render("(new file)"))
			case FileUpdate:
				fmt.Fprintf(w, "      %s %s  %s\n",
					output.Yellow.Render("~"), c.Path, output.Yellow.Render("(content changed)"))
			case FileDrift:
				fmt.Fprintf(w, "      %s %s  %s on_drift: %s → skipping apply\n",
					output.Yellow.Render("⚠"), c.Path, output.Yellow.Render("[drift detected]"), c.OnDrift)
			case FileSkip:
				fmt.Fprintf(w, "      %s %s  %s on_drift: skip → ignored\n",
					output.Dim.Render("-"), c.Path, output.Dim.Render("[drift detected]"))
			}
		}
		fmt.Fprintln(w)
	}
}

// PrintApplyResults prints the results of FileSet apply.
func PrintApplyResults(w io.Writer, results []FileApplyResult) {
	for _, r := range results {
		if r.Skipped {
			fmt.Fprintf(w, "  %s %s %s  drift detected, skipped (on_drift: %s)\n",
				output.Yellow.Render("⚠"), output.Bold.Render(r.Change.Target), r.Change.Path, r.Change.OnDrift)
		} else if r.Err != nil {
			fmt.Fprintf(w, "  %s %s %s: %v\n",
				output.Red.Render("✗"), output.Bold.Render(r.Change.Target), r.Change.Path, r.Err)
		} else {
			fmt.Fprintf(w, "  %s %s %s  %sd\n",
				output.Green.Render("✓"), output.Bold.Render(r.Change.Target), r.Change.Path, r.Change.Type)
		}
	}
}

// HasChanges returns true if any file changes are non-noop.
func HasChanges(changes []FileChange) bool {
	for _, c := range changes {
		if c.Type != FileNoOp && c.Type != FileSkip {
			return true
		}
	}
	return false
}

// CountChanges returns create, update, drift counts.
func CountChanges(changes []FileChange) (creates, updates, drifts int) {
	for _, c := range changes {
		switch c.Type {
		case FileCreate:
			creates++
		case FileUpdate:
			updates++
		case FileDrift:
			drifts++
		}
	}
	return
}

func PrintSummary(w io.Writer, results []FileApplyResult) {
	succeeded := 0
	failed := 0
	skipped := 0
	for _, r := range results {
		if r.Skipped {
			skipped++
		} else if r.Err != nil {
			failed++
		} else {
			succeeded++
		}
	}
	fmt.Fprintf(w, "\nFileSet apply: %s changes applied",
		output.Green.Render(fmt.Sprintf("%d", succeeded)))
	if failed > 0 {
		fmt.Fprintf(w, ", %s failed", output.Red.Render(fmt.Sprintf("%d", failed)))
	}
	if skipped > 0 {
		fmt.Fprintf(w, ", %s skipped (drift)", output.Yellow.Render(fmt.Sprintf("%d", skipped)))
	}
	fmt.Fprintln(w, ".")
}
