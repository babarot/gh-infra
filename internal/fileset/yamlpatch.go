package fileset

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// ReplaceInlineContent replaces the content field of a specific FileEntry
// in a YAML manifest file, preserving formatting and comments.
//
// docIndex is the 0-based index of the YAML document within the file.
// fileIndex is the 0-based index of the file entry within spec.files[].
// newContent is the new content string to set.
func ReplaceInlineContent(yamlBytes []byte, docIndex int, fileIndex int, newContent string) ([]byte, error) {
	file, err := parser.ParseBytes(yamlBytes, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	if docIndex >= len(file.Docs) {
		return nil, fmt.Errorf("document index %d out of range (have %d documents)", docIndex, len(file.Docs))
	}

	path, err := yaml.PathString(fmt.Sprintf("$.spec.files[%d].content", fileIndex))
	if err != nil {
		return nil, fmt.Errorf("build YAML path: %w", err)
	}

	// Create a single-document file for path operations
	singleDoc := &ast.File{Docs: []*ast.DocumentNode{file.Docs[docIndex]}}

	// Build a literal block scalar node for the new content
	newNode := newLiteralNode(newContent)

	if err := path.ReplaceWithNode(singleDoc, newNode); err != nil {
		return nil, fmt.Errorf("replace content node: %w", err)
	}

	// Reassemble the file
	file.Docs[docIndex] = singleDoc.Docs[0]

	return []byte(file.String()), nil
}

// newLiteralNode creates an AST StringNode with literal block scalar style (|).
func newLiteralNode(content string) ast.Node {
	// Ensure content ends with a newline for proper literal block formatting
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	tk := token.Literal(content, content, &token.Position{})
	return ast.String(tk)
}
