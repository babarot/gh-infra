package yamledit

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	goyaml "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
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

// SetLiteral replaces the scalar at yamlPath with content, as a literal block
// scalar (`|`) when content has more than one line and can be held by one,
// and as a plain or quoted scalar otherwise.
//
// The value is replaced in place in the bytes, at the position the parser
// gives for the node, so the rest of the file stays byte for byte as it was:
// re-rendering the document (as Set does) would reformat other values, and
// goccy/go-yaml's ReplaceWithNode corrupts literal block scalars
// (https://github.com/goccy/go-yaml/issues/636). An anchor on the value
// and a comment after it are kept. A value that cannot be located, or
// written so that it reads back as content, is an error.
func SetLiteral(data []byte, docIndex int, yamlPath string, content string) ([]byte, error) {
	ctx, err := newPathContext(data, docIndex, yamlPath)
	if err != nil {
		return nil, err
	}
	node, err := ctx.path.FilterNode(ctx.targetDoc.Body)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("yamledit: %q not found in doc %d", yamlPath, docIndex)
	}
	flow := inFlow(ctx.targetDoc.Body, node, false)
	for {
		switch n := node.(type) {
		case *ast.AnchorNode:
			node = n.Value
			continue
		case *ast.TagNode:
			node = n.Value
			continue
		}
		break
	}
	text := string(data)
	start, err := offsetOf(text, node.GetToken().Position)
	if err != nil {
		return nil, fmt.Errorf("yamledit: %q: %w", yamlPath, err)
	}
	lineStart := strings.LastIndex(text[:start], "\n") + 1

	// The old value runs from start to end, which is past the newline
	// ending its last line (or the end of the file). rest is the remainder
	// of its first line after it (a comment), and blank is the number of
	// blank lines a block scalar ends with, written back as empty lines.
	var end, blank int
	rest := ""
	switch n := node.(type) {
	case *ast.AliasNode:
		// Writing here would change what every other alias of the anchor
		// reads too.
		return nil, fmt.Errorf("yamledit: %q is an alias", yamlPath)
	case *ast.LiteralNode:
		header := strings.Trim(n.GetToken().Origin, yamlSpace)
		if !strings.HasPrefix(text[start:], header) {
			return nil, fmt.Errorf("yamledit: %q: cannot locate block scalar", yamlPath)
		}
		headerEnd := lineEnd(text, start)
		rest = text[start+len(header) : headerEnd]
		end = min(headerEnd+1, len(text))
		lines, blankLines := blockLines(n.Value.GetToken().Origin)
		if !strings.HasPrefix(text[end:], lines) {
			return nil, fmt.Errorf("yamledit: %q: cannot locate block scalar", yamlPath)
		}
		end += len(lines)
		blank = blankLines
		// Spaces at the end of the file are a last blank line of the
		// block, not the indentation of a line after it.
		if end < len(text) && strings.Trim(text[end:], " \t") == "" {
			end = len(text)
			blank++
		}
	default:
		if _, ok := node.(ast.ScalarNode); !ok {
			return nil, fmt.Errorf("yamledit: %q is not a scalar", yamlPath)
		}
		value := strings.Trim(node.GetToken().Origin, yamlSpace)
		if !strings.HasPrefix(text[start:], value) {
			return nil, fmt.Errorf("yamledit: %q: cannot locate scalar", yamlPath)
		}
		rest = text[start+len(value) : lineEnd(text, start+len(value))]
		end = min(start+len(value)+len(rest)+1, len(text))
	}
	after := strings.Repeat("\n", blank) + text[end:]

	var out string
	if block, ok := literalBlockFor(content, flow); ok {
		header, body := block(strings.Repeat(" ", keyColumn(text[lineStart:start])+2))
		out = text[:start] + header + rest + "\n" + body + after
	} else {
		scalar, err := scalarFor(content, flow)
		if err != nil {
			return nil, fmt.Errorf("yamledit: %q: %w", yamlPath, err)
		}
		out = text[:start] + scalar + rest + "\n" + after
	}
	// What was written must read back as content; never leave a file that
	// does not.
	if got, err := readValue([]byte(out), docIndex, ctx.path); err != nil || got != content {
		return nil, fmt.Errorf("yamledit: %q: cannot write the value so that it reads back the same", yamlPath)
	}
	return []byte(out), nil
}

// readValue reads the value at path in document docIndex of data.
func readValue(data []byte, docIndex int, path *goyaml.Path) (any, error) {
	_, doc, err := parseDocument(data, docIndex)
	if err != nil {
		return nil, err
	}
	node, err := path.FilterNode(doc.Body)
	if err != nil || node == nil {
		return nil, fmt.Errorf("not found")
	}
	var v any
	if err := goyaml.NodeToValue(node, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// yamlSpace is the white space YAML separates tokens with: unlike
// strings.TrimSpace, it leaves other Unicode spaces, which are content.
const yamlSpace = " \t\r\n"

// blockLines returns the lines of a block scalar's text in the file, each
// with its newline, and how many of them are blank (spaces only) at the end.
// The parser's text runs on to the indentation of the line after the block,
// which is not one of its lines.
func blockLines(origin string) (string, int) {
	if i := strings.LastIndex(origin, "\n"); i >= 0 && strings.Trim(origin[i+1:], " \t") == "" {
		origin = origin[:i+1]
	}
	blank := 0
	for line := range strings.SplitSeq(strings.TrimSuffix(origin, "\n"), "\n") {
		if strings.Trim(line, " \t") == "" {
			blank++
		} else {
			blank = 0
		}
	}
	if origin == "" {
		blank = 0
	}
	return origin, blank
}

// offsetOf returns the byte offset in text of a parser position (1-based
// line, and column in characters).
func offsetOf(text string, pos *token.Position) (int, error) {
	at := 0
	for line := 1; line < pos.Line; line++ {
		i := strings.IndexByte(text[at:], '\n')
		if i < 0 {
			return 0, fmt.Errorf("line %d is past the end", pos.Line)
		}
		at += i + 1
	}
	for col := 1; col < pos.Column; col++ {
		if at >= len(text) || text[at] == '\n' {
			return 0, fmt.Errorf("column %d is past the end of line %d", pos.Column, pos.Line)
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		at += size
	}
	return at, nil
}

// lineEnd returns the offset of the newline ending the line at i, or the
// end of text.
func lineEnd(text string, i int) int {
	if j := strings.IndexByte(text[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(text)
}

// inFlow reports whether target is inside a flow mapping or sequence
// under node, where a block scalar cannot be written.
func inFlow(node, target ast.Node, flow bool) bool {
	if node == nil {
		return false
	}
	if node == target {
		return flow
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, v := range n.Values {
			if inFlow(v, target, flow || n.IsFlowStyle) {
				return true
			}
		}
	case *ast.MappingValueNode:
		return inFlow(n.Value, target, flow)
	case *ast.SequenceNode:
		for _, v := range n.Values {
			if inFlow(v, target, flow || n.IsFlowStyle) {
				return true
			}
		}
	case *ast.AnchorNode:
		return inFlow(n.Value, target, flow)
	case *ast.TagNode:
		return inFlow(n.Value, target, flow)
	}
	return false
}

// keyColumn returns the column of the key on a line up to its value, e.g.
// 4 for "  - content: " and 6 for "      content: ".
func keyColumn(line string) int {
	i := 0
	for i < len(line) {
		switch {
		case line[i] == ' ':
			i++
		case line[i] == '-' && i+1 < len(line) && line[i+1] == ' ':
			i += 2
		default:
			return i
		}
	}
	return i
}

// literalBlockFor returns a function writing content as a literal block
// with its lines indented by indent, or false when a literal block cannot
// hold content as it is: in a flow collection, without a newline, with a
// first line starting with whitespace (it would be read as indentation), with
// a carriage return or another control character YAML does not keep, with
// more than one trailing newline (keeping them would also take in blank
// lines that follow the value), or without a trailing newline but with
// whitespace at the end.
func literalBlockFor(content string, flow bool) (func(indent string) (string, string), bool) {
	text := strings.TrimSuffix(content, "\n")
	if flow || text == "" || !strings.Contains(content, "\n") || strings.HasSuffix(text, "\n") {
		return nil, false
	}
	if first := strings.TrimLeft(text, "\n"); strings.HasPrefix(first, " ") || strings.HasPrefix(first, "\t") {
		return nil, false
	}
	for _, r := range text {
		if r != '\n' && r != '\t' && r != ' ' && !unicode.IsPrint(r) {
			return nil, false
		}
	}
	if !utf8.ValidString(text) {
		return nil, false
	}
	header := "|"
	if text == content {
		// goccy/go-yaml strips trailing spaces from the last line of a
		// strip-chomped block when it reads it back.
		if strings.HasSuffix(text, " ") || strings.HasSuffix(text, "\t") {
			return nil, false
		}
		header = "|-"
	}
	return func(indent string) (string, string) {
		var b strings.Builder
		for line := range strings.SplitSeq(text, "\n") {
			if line != "" {
				b.WriteString(indent + line)
			}
			b.WriteString("\n")
		}
		return header, b.String()
	}, true
}

// scalarFor returns content as a plain scalar when goccy/go-yaml writes one
// that reads back as content, and as a double-quoted scalar otherwise.
func scalarFor(content string, flow bool) (string, error) {
	if !utf8.ValidString(content) {
		return "", fmt.Errorf("content is not valid UTF-8")
	}
	// goccy/go-yaml drops a tab in a plain scalar when it reads it back,
	// and writes some control characters unquoted.
	plain := !flow && !strings.ContainsFunc(content, func(r rune) bool { return r == '\t' || !unicode.IsPrint(r) && r != ' ' })
	if plain {
		if b, err := goyaml.Marshal(content); err == nil {
			s := strings.TrimSuffix(string(b), "\n")
			var back any
			if !strings.Contains(s, "\n") && goyaml.Unmarshal([]byte(s), &back) == nil && back == content {
				return s, nil
			}
		}
	}
	return strconv.Quote(content), nil
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

	for i, p := range slices.Backward(parents) {
		if childMap, ok := p.child.(*ast.MappingNode); ok && len(childMap.Values) == 0 {
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
