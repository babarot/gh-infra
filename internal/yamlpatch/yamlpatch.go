package yamlpatch

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// ReplaceLiteralContent replaces a YAML node addressed by yamlPath with a literal
// block scalar containing newContent, preserving comments and surrounding formatting.
func ReplaceLiteralContent(yamlBytes []byte, docIndex int, yamlPath, newContent string) ([]byte, error) {
	path, err := yaml.PathString(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("build YAML path: %w", err)
	}
	return replaceLiteralContent(yamlBytes, docIndex, path, newContent)
}

func replaceLiteralContent(yamlBytes []byte, docIndex int, path *yaml.Path, newContent string) ([]byte, error) {
	file, err := parser.ParseBytes(yamlBytes, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	if docIndex >= len(file.Docs) {
		return nil, fmt.Errorf("document index %d out of range (have %d documents)", docIndex, len(file.Docs))
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

// ReplaceYAMLNode replaces a node at the given YAML path within a specific
// document of a multi-document YAML file. The replacement value is provided
// as a Go value that will be marshaled to a YAML AST node.
//
// docIndex is the 0-based document index. yamlPath uses YAMLPath syntax (e.g. "$.spec").
func ReplaceYAMLNode(yamlBytes []byte, docIndex int, yamlPath string, value any) ([]byte, error) {
	file, err := parser.ParseBytes(yamlBytes, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	if docIndex >= len(file.Docs) {
		return nil, fmt.Errorf("document index %d out of range (have %d documents)", docIndex, len(file.Docs))
	}

	p, err := yaml.PathString(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("build YAML path %q: %w", yamlPath, err)
	}

	// Marshal the value to YAML, then parse it as an AST node
	valueBytes, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal replacement value: %w", err)
	}

	valueFile, err := parser.ParseBytes(valueBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("parse replacement YAML: %w", err)
	}
	if len(valueFile.Docs) == 0 || valueFile.Docs[0].Body == nil {
		return nil, fmt.Errorf("empty replacement value")
	}

	singleDoc := &ast.File{Docs: []*ast.DocumentNode{file.Docs[docIndex]}}

	if err := p.ReplaceWithNode(singleDoc, valueFile.Docs[0].Body); err != nil {
		return nil, fmt.Errorf("replace node at %s: %w", yamlPath, err)
	}

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
