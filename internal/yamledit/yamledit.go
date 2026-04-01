package yamledit

import (
	"fmt"

	goyaml "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// ReplaceNode replaces a structured node at the given YAML path in the specified
// document of a (possibly multi-document) YAML byte slice.
// Comments and formatting in unchanged parts are preserved.
func ReplaceNode(data []byte, docIndex int, yamlPath string, value any) ([]byte, error) {
	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("yamledit: parse: %w", err)
	}

	if docIndex < 0 || docIndex >= len(file.Docs) {
		return nil, fmt.Errorf("yamledit: document index %d out of range (have %d docs)", docIndex, len(file.Docs))
	}

	path, err := goyaml.PathString(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("yamledit: invalid path %q: %w", yamlPath, err)
	}

	// Marshal the Go value to YAML, then parse it back to get an AST node.
	valueBytes, err := goyaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("yamledit: marshal value: %w", err)
	}
	valueFile, err := parser.ParseBytes(valueBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("yamledit: parse value: %w", err)
	}
	if len(valueFile.Docs) == 0 || valueFile.Docs[0].Body == nil {
		return nil, fmt.Errorf("yamledit: marshaled value produced empty AST")
	}
	valueNode := valueFile.Docs[0].Body

	// ReplaceWithNode operates on all docs in an ast.File.
	// To target a single document, wrap it in a temporary single-doc file.
	targetDoc := file.Docs[docIndex]
	tmpFile := &ast.File{Docs: []*ast.DocumentNode{targetDoc}}

	if err := path.ReplaceWithNode(tmpFile, valueNode); err != nil {
		return nil, fmt.Errorf("yamledit: replace at %q in doc %d: %w", yamlPath, docIndex, err)
	}

	file.Docs[docIndex] = tmpFile.Docs[0]

	return []byte(file.String()), nil
}

// ReplaceContent replaces a literal block (content: |) at the given YAML path
// in the specified document. Comments are preserved.
func ReplaceContent(data []byte, docIndex int, yamlPath string, content string) ([]byte, error) {
	// For literal blocks, we wrap the content as a YAML string and replace.
	return ReplaceNode(data, docIndex, yamlPath, content)
}
