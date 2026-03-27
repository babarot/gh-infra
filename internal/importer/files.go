package importer

import (
	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
)

type FileChange = fileset.ImportChange

const (
	ImportWriteSource = fileset.ImportWriteSource
	ImportWriteInline = fileset.ImportWriteInline
	ImportSkip        = fileset.ImportSkip
)

func PlanFiles(proc *fileset.Processor, fileSets []*manifest.FileSetDocument, filterRepo string) ([]FileChange, error) {
	return fileset.PlanImport(proc, fileSets, filterRepo)
}

func ApplyFiles(changes []FileChange, manifestBytes map[string][]byte) error {
	return fileset.ApplyImport(changes, manifestBytes)
}

func FileImportSummary(changes []FileChange) (written, unchanged, skipped int) {
	return fileset.ImportSummary(changes)
}

func HasFileImportChanges(changes []FileChange) bool {
	return fileset.HasImportChanges(changes)
}
