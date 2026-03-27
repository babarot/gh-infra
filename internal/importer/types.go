package importer

import (
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
)

type Target struct {
	Owner string
	Name  string
}

func (t Target) FullName() string {
	return t.Owner + "/" + t.Name
}

type Matches struct {
	Repositories   []*manifest.RepositoryDocument
	RepositorySets []*manifest.RepositoryDocument
	FileSets       []*manifest.FileSetDocument
}

func (m Matches) HasRepo() bool {
	return len(m.Repositories) > 0 || len(m.RepositorySets) > 0
}

func (m Matches) HasFiles() bool {
	return len(m.FileSets) > 0
}

func (m Matches) IsEmpty() bool {
	return !m.HasRepo() && !m.HasFiles()
}

type RepoPlan struct {
	Changes       []repository.Change
	ManifestEdits map[string][]byte
	UpdatedDocs   int
}

type IntoPlan struct {
	RepoChanges   []repository.Change
	FileChanges   []FileImportChange
	ManifestEdits map[string][]byte
	UpdatedDocs   int

	matches       Matches
	manifestBytes map[string][]byte
	readManifest  func(string) error
}

func (p *IntoPlan) AddRepoPlan(repoPlan RepoPlan) {
	p.RepoChanges = append(p.RepoChanges, repoPlan.Changes...)
	for path, data := range repoPlan.ManifestEdits {
		p.ManifestEdits[path] = data
	}
	p.UpdatedDocs += repoPlan.UpdatedDocs
}

func (p IntoPlan) HasChanges() bool {
	return repository.HasRealChanges(p.RepoChanges) || HasFileImportChanges(p.FileChanges)
}

func (p IntoPlan) Summary() (written, unchanged, skipped int) {
	written, unchanged, skipped = FileImportSummary(p.FileChanges)
	written += p.UpdatedDocs
	return
}

func (p IntoPlan) TotalWritten() int {
	written, _, _ := p.Summary()
	return written
}
