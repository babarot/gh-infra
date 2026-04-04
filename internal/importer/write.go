package importer

import (
	"fmt"
	"os"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/yamledit"
)

// Write writes the planned changes to disk.
// It applies manifest edits (repo spec patches) and file changes.
func Write(plan *Result) error {
	// Apply manifest edits (repo/reposet YAML patches).
	for path, data := range plan.ManifestEdits {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("write manifest %s: %w", path, err)
		}
	}

	// Apply file changes.
	for _, c := range plan.FileChanges {
		if c.Type != fileset.ChangeUpdate {
			continue
		}

		switch c.WriteMode {
		case WriteSource:
			if err := os.WriteFile(c.LocalTarget, []byte(c.Desired), 0644); err != nil {
				return fmt.Errorf("write source %s: %w", c.LocalTarget, err)
			}

		case WriteInline:
			// Read current manifest (may already be patched by repo edits).
			data, ok := plan.ManifestEdits[c.ManifestPath]
			if !ok {
				var err error
				data, err = os.ReadFile(c.ManifestPath)
				if err != nil {
					return fmt.Errorf("read manifest for inline edit %s: %w", c.ManifestPath, err)
				}
			}
			updated, err := yamledit.ReplaceContent(data, c.DocIndex, c.YAMLPath, c.Desired)
			if err != nil {
				return fmt.Errorf("inline edit %s in %s: %w", c.YAMLPath, c.ManifestPath, err)
			}
			plan.ManifestEdits[c.ManifestPath] = updated
			if err := os.WriteFile(c.ManifestPath, updated, 0644); err != nil {
				return fmt.Errorf("write manifest %s: %w", c.ManifestPath, err)
			}

		case WriteSkip:
			// Nothing to do.
		}
	}

	return nil
}
