package yamledit

import (
	"fmt"
	"strconv"
	"strings"

	goyaml "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// Set replaces the node at yamlPath in the given document.
// Comments and formatting in unchanged parts are preserved.
func Set(data []byte, docIndex int, yamlPath string, value any, opts ...goyaml.EncodeOption) ([]byte, error) {
	ctx, err := newPathContext(data, docIndex, yamlPath)
	if err != nil {
		return nil, err
	}
	valueNode, err := marshalToNode(value, opts...)
	if err != nil {
		return nil, err
	}
	tmp := ctx.tmpFile()
	if err := ctx.path.ReplaceWithNode(tmp, valueNode); err != nil {
		return nil, fmt.Errorf("yamledit: set at %q in doc %d: %w", yamlPath, docIndex, err)
	}
	ctx.targetDoc = tmp.Docs[0]
	return ctx.bytes(), nil
}

// SetLiteral replaces the node at yamlPath, rendering multiline strings as a
// literal block scalar (`|`).
//
// goccy/go-yaml's ReplaceWithNode corrupts literal block scalars that contain
// YAML-special characters (*, !, etc.) at the start of lines. To work around
// this, if the existing node is already a literal block scalar we replace the
// content directly in the byte stream, preserving the original indentation.
func SetLiteral(data []byte, docIndex int, yamlPath string, content string) ([]byte, error) {
	ctx, err := newPathContext(data, docIndex, yamlPath)
	if err != nil {
		return nil, err
	}
	node, err := ctx.path.ReadNode(ctx.tmpFile())
	if err != nil {
		return nil, err
	}

	if ln, ok := node.(*ast.LiteralNode); ok {
		return replaceLiteralContent(data, ln, content)
	}

	// Single-line content doesn't need a literal block scalar, and
	// ReplaceWithNode handles plain/quoted scalars correctly.
	if len(strings.Split(strings.TrimRight(content, "\n"), "\n")) <= 1 {
		return Set(data, docIndex, yamlPath, content)
	}

	// For non-LiteralNode (StringNode etc.) with multiline content, replace
	// the entire node with a literal block scalar via direct byte replacement.
	// Using ReplaceWithNode would corrupt the content (see replaceLiteralContent comment).
	return replaceWithLiteralBlock(data, node, content)
}

// replaceLiteralContent replaces a literal block scalar's content directly in
// the byte stream, avoiding goccy/go-yaml's broken ReplaceWithNode for block scalars.
func replaceLiteralContent(data []byte, ln *ast.LiteralNode, newContent string) ([]byte, error) {
	origin := ln.Value.GetToken().Origin
	idx := strings.Index(string(data), origin)
	if idx < 0 {
		return nil, fmt.Errorf("yamledit: cannot locate literal block content in source")
	}

	// Detect indentation from the original content lines
	indent := ""
	for _, line := range strings.Split(origin, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed != "" {
			indent = line[:len(line)-len(trimmed)]
			break
		}
	}

	// Build replacement with same indentation
	lines := strings.Split(strings.TrimRight(newContent, "\n"), "\n")
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(indent)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	// Preserve trailing whitespace from original (indent before next key)
	parts := strings.Split(origin, "\n")
	sb.WriteString(parts[len(parts)-1])

	result := string(data[:idx]) + sb.String() + string(data[idx+len(origin):])
	return []byte(result), nil
}

// replaceWithLiteralBlock replaces any scalar node with a literal block scalar
// via direct byte replacement. This avoids ReplaceWithNode which corrupts
// LiteralNode content due to an AddColumn/Origin mismatch in goccy/go-yaml.
func replaceWithLiteralBlock(data []byte, node ast.Node, newContent string) ([]byte, error) {
	origin := node.GetToken().Origin
	idx := strings.Index(string(data), origin)
	if idx < 0 {
		return nil, fmt.Errorf("yamledit: cannot locate node origin in source")
	}

	// The Origin may have leading whitespace (e.g., " " between "- " and the
	// value in a sequence). Preserve it so the replacement stays syntactically
	// valid (e.g., "- |" not "-|").
	leadingSpace := ""
	for i, c := range origin {
		if c != ' ' && c != '\t' {
			leadingSpace = origin[:i]
			break
		}
	}

	// Determine content indentation by looking at the line containing this node.
	// For `content: old-a`, the key indent is the number of leading spaces before
	// `content:`. Content lines inside a literal block are indented key_indent + 2.
	lineStart := strings.LastIndex(string(data[:idx]), "\n") + 1
	line := string(data[lineStart:idx])
	keyIndent := len(line) - len(strings.TrimLeft(line, " \t"))
	contentIndent := strings.Repeat(" ", keyIndent+2)

	// Build literal block scalar: "<leading>|\n" + indented content lines
	lines := strings.Split(strings.TrimRight(newContent, "\n"), "\n")
	var sb strings.Builder
	sb.WriteString(leadingSpace)
	sb.WriteString("|\n")
	for _, line := range lines {
		sb.WriteString(contentIndent)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	result := string(data[:idx]) + sb.String() + string(data[idx+len(origin):])
	return []byte(result), nil
}

// Merge merges a mapping node at yamlPath in the given document. This is useful
// for updating only changed fields while preserving untouched siblings.
func Merge(data []byte, docIndex int, yamlPath string, value any, opts ...goyaml.EncodeOption) ([]byte, error) {
	ctx, err := newPathContext(data, docIndex, yamlPath)
	if err != nil {
		return nil, err
	}
	valueNode, err := marshalToNode(value, opts...)
	if err != nil {
		return nil, err
	}
	tmp := ctx.tmpFile()
	if err := ctx.path.MergeFromNode(tmp, valueNode); err != nil {
		return nil, fmt.Errorf("yamledit: merge at %q in doc %d: %w", yamlPath, docIndex, err)
	}
	ctx.targetDoc = tmp.Docs[0]
	return ctx.bytes(), nil
}

// Delete removes the node at yamlPath in the specified document.
// If the deletion leaves parent mappings empty, those parents are pruned too.
func Delete(data []byte, docIndex int, yamlPath string) ([]byte, error) {
	ctx, err := newSimplePathContext(data, docIndex, yamlPath)
	if err != nil {
		return nil, err
	}
	if len(ctx.segs) == 0 {
		return nil, fmt.Errorf("yamledit: invalid path %q", yamlPath)
	}

	type parentRef struct {
		node  *ast.MappingNode
		key   string
		child ast.Node
	}

	current := ctx.targetDoc.Body
	var parents []parentRef

	for i, seg := range ctx.segs[:len(ctx.segs)-1] {
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

	last := ctx.segs[len(ctx.segs)-1]
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

	return ctx.bytes(), nil
}

// Exists reports whether yamlPath exists in the given document.
func Exists(data []byte, docIndex int, yamlPath string) (bool, error) {
	ctx, err := newSimplePathContext(data, docIndex, yamlPath)
	if err != nil {
		return false, err
	}

	current := ctx.targetDoc.Body
	for _, seg := range ctx.segs {
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

type pathContext struct {
	file      *ast.File
	targetDoc *ast.DocumentNode
	path      *goyaml.Path
	yamlPath  string
	docIndex  int
}

func newPathContext(data []byte, docIndex int, yamlPath string) (*pathContext, error) {
	file, targetDoc, err := parseDocument(data, docIndex)
	if err != nil {
		return nil, err
	}
	path, err := goyaml.PathString(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("yamledit: invalid path %q: %w", yamlPath, err)
	}
	return &pathContext{
		file:      file,
		targetDoc: targetDoc,
		path:      path,
		yamlPath:  yamlPath,
		docIndex:  docIndex,
	}, nil
}

func (c *pathContext) tmpFile() *ast.File {
	return &ast.File{Docs: []*ast.DocumentNode{c.targetDoc}}
}

func (c *pathContext) bytes() []byte {
	c.file.Docs[c.docIndex] = c.targetDoc
	return []byte(c.file.String())
}

type simplePathContext struct {
	*pathContext
	segs []pathSegment
}

func newSimplePathContext(data []byte, docIndex int, yamlPath string) (*simplePathContext, error) {
	ctx, err := newPathContext(data, docIndex, yamlPath)
	if err != nil {
		return nil, err
	}
	segs, err := parseSimplePath(yamlPath)
	if err != nil {
		return nil, err
	}
	return &simplePathContext{pathContext: ctx, segs: segs}, nil
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
