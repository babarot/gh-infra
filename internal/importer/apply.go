package importer

import (
	"fmt"
	"os"
)

// ApplyInto writes the planned manifest edits to disk.
func ApplyInto(plan *IntoPlan) error {
	for path, data := range plan.ManifestEdits {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}
