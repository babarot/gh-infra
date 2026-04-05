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
			updated, err := yamledit.SetLiteral(data, c.DocIndex, c.YAMLPath, c.Desired)
			if err != nil {
				return fmt.Errorf("inline edit %s in %s: %w", c.YAMLPath, c.ManifestPath, err)
			}
			plan.ManifestEdits[c.ManifestPath] = updated
			if err := os.WriteFile(c.ManifestPath, updated, 0644); err != nil {
				return fmt.Errorf("write manifest %s: %w", c.ManifestPath, err)
			}

		case WritePatch:
			if c.PatchEntry == nil {
				return fmt.Errorf("WritePatch for %s but PatchEntry is nil", c.Path)
			}
			// Read current manifest (may already be patched by repo edits).
			data, ok := plan.ManifestEdits[c.ManifestPath]
			if !ok {
				var err error
				data, err = os.ReadFile(c.ManifestPath)
				if err != nil {
					return fmt.Errorf("read manifest for patch edit %s: %w", c.ManifestPath, err)
				}
			}
			updated, err := writePatchField(data, c)
			if err != nil {
				return fmt.Errorf("patch edit %s in %s: %w", c.YAMLPath, c.ManifestPath, err)
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

func writePatchField(data []byte, c Change) ([]byte, error) {
	patchesPath := c.YAMLPath + ".patches"

	if c.PatchContent == "" {
		return yamledit.Delete(data, c.DocIndex, patchesPath)
	}

	exists, err := yamledit.Exists(data, c.DocIndex, patchesPath)
	if err != nil {
		return nil, err
	}

	var updated []byte
	if exists {
		updated, err = yamledit.Set(data, c.DocIndex, patchesPath, []string{c.PatchContent})
		if err != nil {
			return nil, err
		}
	} else {
		updated, err = yamledit.Merge(data, c.DocIndex, c.YAMLPath, map[string]any{
			"patches": []string{c.PatchContent},
		})
		if err != nil {
			return nil, err
		}
	}

	patchContentPath := patchesPath + "[0]"
	updated, err = yamledit.SetLiteral(updated, c.DocIndex, patchContentPath, c.PatchContent)
	if err != nil {
		return nil, err
	}
	return updated, nil
}
