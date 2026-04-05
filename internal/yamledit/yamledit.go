package yamledit

import (
	"fmt"
	"strconv"
	"strings"

	goyaml "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// ReplaceNode replaces a structured node at the given YAML path in the specified
// document of a (possibly multi-document) YAML byte slice.
// Comments and formatting in unchanged parts are preserved.
func ReplaceNode(data []byte, docIndex int, yamlPath string, value any, opts ...goyaml.EncodeOption) ([]byte, error) {
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
	valueBytes, err := goyaml.MarshalWithOptions(value, opts...)
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
// Multiline strings are always rendered as literal block scalars (|).
func ReplaceContent(data []byte, docIndex int, yamlPath string, content string) ([]byte, error) {
	return ReplaceNode(data, docIndex, yamlPath, content, goyaml.UseLiteralStyleIfMultiline(true))
}

// MergeNode merges a mapping node at the given YAML path in the specified
// document. This is useful for updating only changed fields while preserving
// ordering and formatting of untouched siblings.
func MergeNode(data []byte, docIndex int, yamlPath string, value any, opts ...goyaml.EncodeOption) ([]byte, error) {
	file, targetDoc, err := parseDocument(data, docIndex)
	if err != nil {
		return nil, err
	}

	path, err := goyaml.PathString(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("yamledit: invalid path %q: %w", yamlPath, err)
	}

	valueNode, err := marshalToNode(value, opts...)
	if err != nil {
		return nil, err
	}

	tmpFile := &ast.File{Docs: []*ast.DocumentNode{targetDoc}}
	if err := path.MergeFromNode(tmpFile, valueNode); err != nil {
		return nil, fmt.Errorf("yamledit: merge at %q in doc %d: %w", yamlPath, docIndex, err)
	}

	file.Docs[docIndex] = tmpFile.Docs[0]
	return []byte(file.String()), nil
}

// DeleteNode removes the node at the given YAML path in the specified document.
// If the deletion leaves parent mappings empty, those parents are pruned too.
func DeleteNode(data []byte, docIndex int, yamlPath string) ([]byte, error) {
	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("yamledit: parse: %w", err)
	}

	if docIndex < 0 || docIndex >= len(file.Docs) {
		return nil, fmt.Errorf("yamledit: document index %d out of range (have %d docs)", docIndex, len(file.Docs))
	}

	segs, err := parseSimplePath(yamlPath)
	if err != nil {
		return nil, err
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("yamledit: invalid path %q", yamlPath)
	}

	type parentRef struct {
		node  *ast.MappingNode
		key   string
		child ast.Node
	}

	current := file.Docs[docIndex].Body
	var parents []parentRef

	for i, seg := range segs[:len(segs)-1] {
		mapping, ok := current.(*ast.MappingNode)
		if !ok {
			return nil, fmt.Errorf("yamledit: path %q traverses non-mapping node at segment %d", yamlPath, i)
		}
		mv := findMappingValue(mapping, seg.key)
		if mv == nil {
			return data, nil
		}
		child := mv.Value
		if seg.hasIndex {
			seq, ok := child.(*ast.SequenceNode)
			if !ok || seg.index >= len(seq.Values) {
				return data, nil
			}
			child = seq.Values[seg.index]
		}
		parents = append(parents, parentRef{node: mapping, key: seg.key, child: child})
		current = child
	}

	last := segs[len(segs)-1]
	mapping, ok := current.(*ast.MappingNode)
	if !ok {
		return nil, fmt.Errorf("yamledit: path %q traverses non-mapping node at final parent", yamlPath)
	}
	if !deleteMappingKey(mapping, last.key) {
		return data, nil
	}

	for i := len(parents) - 1; i >= 0; i-- {
		if childMap, ok := parents[i].child.(*ast.MappingNode); ok && len(childMap.Values) == 0 {
			deleteMappingKey(parents[i].node, parents[i].key)
			continue
		}
		break
	}

	return []byte(file.String()), nil
}

// PathExists reports whether the given simple YAML path exists in the document.
func PathExists(data []byte, docIndex int, yamlPath string) (bool, error) {
	file, targetDoc, err := parseDocument(data, docIndex)
	if err != nil {
		return false, err
	}
	_ = file

	segs, err := parseSimplePath(yamlPath)
	if err != nil {
		return false, err
	}

	current := targetDoc.Body
	for _, seg := range segs {
		mapping, ok := current.(*ast.MappingNode)
		if !ok {
			return false, nil
		}
		mv := findMappingValue(mapping, seg.key)
		if mv == nil {
			return false, nil
		}
		current = mv.Value
		if seg.hasIndex {
			seq, ok := current.(*ast.SequenceNode)
			if !ok || seg.index >= len(seq.Values) {
				return false, nil
			}
			current = seq.Values[seg.index]
		}
	}
	return true, nil
}

func parseDocument(data []byte, docIndex int) (*ast.File, *ast.DocumentNode, error) {
	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("yamledit: parse: %w", err)
	}

	if docIndex < 0 || docIndex >= len(file.Docs) {
		return nil, nil, fmt.Errorf("yamledit: document index %d out of range (have %d docs)", docIndex, len(file.Docs))
	}
	return file, file.Docs[docIndex], nil
}

func marshalToNode(value any, opts ...goyaml.EncodeOption) (ast.Node, error) {
	valueBytes, err := goyaml.MarshalWithOptions(value, opts...)
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
	return valueFile.Docs[0].Body, nil
}

type pathSegment struct {
	key      string
	hasIndex bool
	index    int
}

func parseSimplePath(yamlPath string) ([]pathSegment, error) {
	if !strings.HasPrefix(yamlPath, "$.") {
		return nil, fmt.Errorf("yamledit: unsupported path %q", yamlPath)
	}
	parts := strings.Split(strings.TrimPrefix(yamlPath, "$."), ".")
	segs := make([]pathSegment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("yamledit: invalid path %q", yamlPath)
		}
		seg := pathSegment{key: part}
		if idx := strings.Index(part, "["); idx >= 0 {
			if !strings.HasSuffix(part, "]") {
				return nil, fmt.Errorf("yamledit: invalid indexed path segment %q", part)
			}
			n, err := strconv.Atoi(part[idx+1 : len(part)-1])
			if err != nil {
				return nil, fmt.Errorf("yamledit: invalid index in path segment %q: %w", part, err)
			}
			seg.key = part[:idx]
			seg.hasIndex = true
			seg.index = n
		}
		segs = append(segs, seg)
	}
	return segs, nil
}

func findMappingValue(node *ast.MappingNode, key string) *ast.MappingValueNode {
	for _, value := range node.Values {
		if mappingKeyName(value) == key {
			return value
		}
	}
	return nil
}

func deleteMappingKey(node *ast.MappingNode, key string) bool {
	for i, value := range node.Values {
		if mappingKeyName(value) != key {
			continue
		}
		node.Values = append(node.Values[:i], node.Values[i+1:]...)
		return true
	}
	return false
}

func mappingKeyName(value *ast.MappingValueNode) string {
	s := strings.TrimSpace(value.Key.String())
	return strings.TrimSuffix(s, ":")
}
