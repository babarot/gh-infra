package importer

import (
	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
)

type FileImportChange = fileset.FileImportChange

const (
	ImportWriteSource = fileset.ImportWriteSource
	ImportWriteInline = fileset.ImportWriteInline
	ImportSkip        = fileset.ImportSkip
)

func PlanFiles(proc *fileset.Processor, fileSets []*manifest.FileSetDocument, filterRepo string) ([]FileImportChange, error) {
	return fileset.PlanImport(proc, fileSets, filterRepo)
}

func ApplyFiles(changes []FileImportChange, manifestBytes map[string][]byte) error {
	return fileset.ApplyImport(changes, manifestBytes)
}

func FileImportSummary(changes []FileImportChange) (written, unchanged, skipped int) {
	return fileset.ImportSummary(changes)
}

func HasFileImportChanges(changes []FileImportChange) bool {
	return fileset.HasImportChanges(changes)
}
