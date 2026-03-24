package manifest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const githubScheme = "github://"

// DefaultResolver is the package-level resolver used by the parser.
// Set RunGH before parsing to enable GitHub source support.
var DefaultResolver = &SourceResolver{}

// SourceResolver resolves file sources (local files, directories, GitHub URLs).
type SourceResolver struct {
	// RunGH executes a gh CLI command and returns stdout.
	// Set by the caller to avoid importing the gh package.
	RunGH func(args ...string) ([]byte, error)
}

// ResolveFiles expands source references in FileSet entries.
func (r *SourceResolver) ResolveFiles(files []FileEntry, yamlDir string) ([]FileEntry, error) {
	var resolved []FileEntry
	for _, entry := range files {
		if entry.Source == "" || entry.Content != "" {
			resolved = append(resolved, entry)
			continue
		}

		if strings.HasPrefix(entry.Source, githubScheme) {
			entries, err := r.resolveGitHub(entry.Source, entry.Path)
			if err != nil {
				return nil, fmt.Errorf("resolve %s: %w", entry.Source, err)
			}
			// Preserve metadata from the original entry
			isDir := len(entries) > 1 || strings.HasSuffix(entry.Source, "/")
			for i := range entries {
				entries[i].Vars = entry.Vars
				entries[i].SyncMode = entry.SyncMode
				entries[i].OnDrift = entry.OnDrift
				if isDir {
					entries[i].DirScope = entry.Path
				}
			}
			resolved = append(resolved, entries...)
		} else {
			entries, err := resolveLocal(entry.Source, entry.Path, yamlDir)
			if err != nil {
				return nil, fmt.Errorf("resolve %s: %w", entry.Source, err)
			}
			// Preserve metadata from the original entry
			isDir := len(entries) > 1 || strings.HasSuffix(entry.Source, "/")
			for i := range entries {
				entries[i].Vars = entry.Vars
				entries[i].SyncMode = entry.SyncMode
				entries[i].OnDrift = entry.OnDrift
				if isDir {
					entries[i].DirScope = entry.Path
				}
			}
			resolved = append(resolved, entries...)
		}
	}
	// Check for duplicate paths after source expansion
	if err := checkDuplicatePaths(resolved); err != nil {
		return nil, err
	}
	return resolved, nil
}

// checkDuplicatePaths returns an error if any file path appears more than once.
// This catches overlapping source directories (e.g. ".github" and ".github/workflows/").
func checkDuplicatePaths(files []FileEntry) error {
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		if seen[f.Path] {
			return fmt.Errorf("duplicate file path %q (check for overlapping source directories)", f.Path)
		}
		seen[f.Path] = true
	}
	return nil
}

// resolveLocal handles local file and directory sources.
func resolveLocal(source, destPath, yamlDir string) ([]FileEntry, error) {
	srcPath := source
	if !filepath.IsAbs(srcPath) {
		srcPath = filepath.Join(yamlDir, srcPath)
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return expandDir(srcPath, destPath)
	}
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, err
	}
	return []FileEntry{{Path: destPath, Content: string(content)}}, nil
}

// parseGitHubSource parses "github://owner/repo/path@ref" into components.
func parseGitHubSource(source string) (owner, repo, path, ref string, err error) {
	s := strings.TrimPrefix(source, githubScheme)

	// Split off @ref if present
	ref = "" // default branch
	if idx := strings.LastIndex(s, "@"); idx != -1 {
		ref = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.SplitN(s, "/", 3)
	if len(parts) < 2 {
		return "", "", "", "", fmt.Errorf("invalid github source %q (expected github://owner/repo/path)", source)
	}
	owner = parts[0]
	repo = parts[1]
	if len(parts) == 3 {
		path = parts[2]
	}
	return owner, repo, path, ref, nil
}

// resolveGitHub fetches file(s) from a GitHub repository via Contents API.
func (r *SourceResolver) resolveGitHub(source, destPath string) ([]FileEntry, error) {
	if r.RunGH == nil {
		return nil, fmt.Errorf("GitHub source %q requires gh CLI", source)
	}

	owner, repo, path, ref, err := parseGitHubSource(source)
	if err != nil {
		return nil, err
	}

	isDir := strings.HasSuffix(path, "/")
	path = strings.TrimSuffix(path, "/")

	endpoint := fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, path)
	if ref != "" {
		endpoint += "?ref=" + ref
	}

	out, err := r.RunGH("api", endpoint)
	if err != nil {
		return nil, fmt.Errorf("fetch %s/%s/%s: %w", owner, repo, path, err)
	}

	if isDir {
		return r.resolveGitHubDir(out, owner, repo, path, ref, destPath)
	}
	return r.resolveGitHubFile(out, destPath)
}

// resolveGitHubFile parses a single file response from Contents API.
func (r *SourceResolver) resolveGitHubFile(data []byte, destPath string) ([]FileEntry, error) {
	var file struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse file response: %w", err)
	}
	content := file.Content
	if file.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("decode base64: %w", err)
		}
		content = string(decoded)
	}
	return []FileEntry{{Path: destPath, Content: content}}, nil
}

// resolveGitHubDir lists a directory and fetches each file.
func (r *SourceResolver) resolveGitHubDir(data []byte, owner, repo, dirPath, ref, destPrefix string) ([]FileEntry, error) {
	var items []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"` // "file" or "dir"
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse directory response: %w", err)
	}

	var entries []FileEntry
	for _, item := range items {
		rel, _ := filepath.Rel(dirPath, item.Path)
		destPath := filepath.ToSlash(filepath.Join(destPrefix, rel))

		if item.Type == "dir" {
			// Recurse into subdirectory
			subSource := fmt.Sprintf("%s%s/%s/%s/", githubScheme, owner, repo, item.Path)
			if ref != "" {
				subSource = fmt.Sprintf("%s%s/%s/%s/@%s", githubScheme, owner, repo, item.Path, ref)
				// Fix: directory with ref
				subSource = fmt.Sprintf("%s%s/%s/%s/", githubScheme, owner, repo, item.Path)
			}
			subEntries, err := r.resolveGitHub(subSource, destPath)
			if err != nil {
				return nil, err
			}
			entries = append(entries, subEntries...)
		} else {
			// Fetch file content
			endpoint := fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, item.Path)
			if ref != "" {
				endpoint += "?ref=" + ref
			}
			out, err := r.RunGH("api", endpoint)
			if err != nil {
				return nil, fmt.Errorf("fetch %s: %w", item.Path, err)
			}
			fileEntries, err := r.resolveGitHubFile(out, destPath)
			if err != nil {
				return nil, err
			}
			entries = append(entries, fileEntries...)
		}
	}
	return entries, nil
}
