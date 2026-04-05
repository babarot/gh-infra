package importer

import (
	"fmt"
	"os"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/yamledit"
)

type editKind string

const (
	editSet        editKind = "set"
	editSetLiteral editKind = "set_literal"
	editMerge      editKind = "merge"
	editDelete     editKind = "delete"
)

type editOp struct {
	kind     editKind
	docIndex int
	path     string
	value    any
}

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
		mode := c.EffectiveWriteMode()
		if err := c.ValidateWriteMode(); err != nil {
			return err
		}
		if c.Type != fileset.ChangeUpdate || mode == WriteSkip {
			continue
		}

		switch mode {
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
			updated, err := applyEditOps(data, []editOp{{
				kind:     editSetLiteral,
				docIndex: c.DocIndex,
				path:     c.YAMLPath,
				value:    c.Desired,
			}})
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
			updated, err := applyPatchChange(data, c)
			if err != nil {
				return fmt.Errorf("patch edit %s in %s: %w", c.PatchYAMLPath, c.ManifestPath, err)
			}
			plan.ManifestEdits[c.ManifestPath] = updated
			if err := os.WriteFile(c.ManifestPath, updated, 0644); err != nil {
				return fmt.Errorf("write manifest %s: %w", c.ManifestPath, err)
			}
		}
	}

	return nil
}

func applyPatchChange(data []byte, c Change) ([]byte, error) {
	ops, err := patchEditOps(data, c)
	if err != nil {
		return nil, err
	}
	return applyEditOps(data, ops)
}

func patchEditOps(data []byte, c Change) ([]editOp, error) {
	basePath := c.PatchYAMLPath
	if basePath == "" {
		basePath = c.YAMLPath
	}
	patchesPath := basePath + ".patches"

	if c.PatchContent == "" {
		return []editOp{{
			kind:     editDelete,
			docIndex: c.DocIndex,
			path:     patchesPath,
		}}, nil
	}

	exists, err := yamledit.Exists(data, c.DocIndex, patchesPath)
	if err != nil {
		return nil, err
	}

	var ops []editOp
	if exists {
		ops = append(ops, editOp{
			kind:     editSet,
			docIndex: c.DocIndex,
			path:     patchesPath,
			value:    []string{c.PatchContent},
		})
	} else {
		ops = append(ops, editOp{
			kind:     editMerge,
			docIndex: c.DocIndex,
			path:     basePath,
			value: map[string]any{
				"patches": []string{c.PatchContent},
			},
		})
	}

	patchContentPath := patchesPath + "[0]"
	ops = append(ops, editOp{
		kind:     editSetLiteral,
		docIndex: c.DocIndex,
		path:     patchContentPath,
		value:    c.PatchContent,
	})
	return ops, nil
}

func applyEditOps(data []byte, ops []editOp) ([]byte, error) {
	var err error
	updated := data
	for _, op := range ops {
		switch op.kind {
		case editSet:
			updated, err = yamledit.Set(updated, op.docIndex, op.path, op.value)
		case editSetLiteral:
			content, ok := op.value.(string)
			if !ok {
				return nil, fmt.Errorf("edit %s at %s requires string value", op.kind, op.path)
			}
			updated, err = yamledit.SetLiteral(updated, op.docIndex, op.path, content)
		case editMerge:
			updated, err = yamledit.Merge(updated, op.docIndex, op.path, op.value)
		case editDelete:
			updated, err = yamledit.Delete(updated, op.docIndex, op.path)
		default:
			return nil, fmt.Errorf("unknown edit op kind %q", op.kind)
		}
		if err != nil {
			return nil, err
		}
	}
	return updated, nil
}
