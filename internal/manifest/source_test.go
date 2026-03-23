package manifest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitHubSource(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantOwner string
		wantRepo  string
		wantPath  string
		wantRef   string
		wantErr   bool
	}{
		{
			name:      "full path with ref",
			source:    "github://octocat/hello-world/src/main.go@v1.0",
			wantOwner: "octocat",
			wantRepo:  "hello-world",
			wantPath:  "src/main.go",
			wantRef:   "v1.0",
		},
		{
			name:      "full path without ref",
			source:    "github://octocat/hello-world/src/main.go",
			wantOwner: "octocat",
			wantRepo:  "hello-world",
			wantPath:  "src/main.go",
			wantRef:   "",
		},
		{
			name:      "repo only no path",
			source:    "github://octocat/hello-world",
			wantOwner: "octocat",
			wantRepo:  "hello-world",
			wantPath:  "",
			wantRef:   "",
		},
		{
			name:      "repo with ref no path",
			source:    "github://octocat/hello-world@main",
			wantOwner: "octocat",
			wantRepo:  "hello-world",
			wantPath:  "",
			wantRef:   "main",
		},
		{
			name:      "nested path with ref",
			source:    "github://org/repo/deep/nested/path/file.txt@feature-branch",
			wantOwner: "org",
			wantRepo:  "repo",
			wantPath:  "deep/nested/path/file.txt",
			wantRef:   "feature-branch",
		},
		{
			name:    "invalid source missing repo",
			source:  "github://octocat",
			wantErr: true,
		},
		{
			name:    "invalid source empty",
			source:  "github://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, path, ref, err := parseGitHubSource(tt.source)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner: got %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo: got %q, want %q", repo, tt.wantRepo)
			}
			if path != tt.wantPath {
				t.Errorf("path: got %q, want %q", path, tt.wantPath)
			}
			if ref != tt.wantRef {
				t.Errorf("ref: got %q, want %q", ref, tt.wantRef)
			}
		})
	}
}

func TestResolveGitHubFile(t *testing.T) {
	r := &SourceResolver{}

	t.Run("base64 encoded content", func(t *testing.T) {
		content := "hello world\n"
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		data, _ := json.Marshal(struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}{
			Content:  encoded,
			Encoding: "base64",
		})

		entries, err := r.resolveGitHubFile(data, "dest/file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Path != "dest/file.txt" {
			t.Errorf("path: got %q, want %q", entries[0].Path, "dest/file.txt")
		}
		if entries[0].Content != content {
			t.Errorf("content: got %q, want %q", entries[0].Content, content)
		}
	})

	t.Run("base64 with newlines", func(t *testing.T) {
		content := "line1\nline2\nline3\n"
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		// Insert newlines as GitHub API does
		withNewlines := encoded[:10] + "\n" + encoded[10:]
		data, _ := json.Marshal(struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}{
			Content:  withNewlines,
			Encoding: "base64",
		})

		entries, err := r.resolveGitHubFile(data, "file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entries[0].Content != content {
			t.Errorf("content: got %q, want %q", entries[0].Content, content)
		}
	})

	t.Run("non-base64 encoding", func(t *testing.T) {
		data, _ := json.Marshal(struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}{
			Content:  "raw content here",
			Encoding: "utf-8",
		})

		entries, err := r.resolveGitHubFile(data, "file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entries[0].Content != "raw content here" {
			t.Errorf("content: got %q, want %q", entries[0].Content, "raw content here")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := r.resolveGitHubFile([]byte("not json"), "file.txt")
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func TestResolveGitHub_NilRunGH(t *testing.T) {
	r := &SourceResolver{RunGH: nil}
	_, err := r.resolveGitHub("github://owner/repo/file.txt", "dest.txt")
	if err == nil {
		t.Fatal("expected error when RunGH is nil")
	}
}

func TestResolveGitHub_SingleFile(t *testing.T) {
	content := "file content"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	fileResp, _ := json.Marshal(struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}{
		Content:  encoded,
		Encoding: "base64",
	})

	r := &SourceResolver{
		RunGH: func(args ...string) ([]byte, error) {
			return fileResp, nil
		},
	}

	entries, err := r.resolveGitHub("github://owner/repo/path/file.go", "dest/file.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Content != content {
		t.Errorf("content: got %q, want %q", entries[0].Content, content)
	}
}

func TestResolveGitHub_WithRef(t *testing.T) {
	var calledEndpoint string
	r := &SourceResolver{
		RunGH: func(args ...string) ([]byte, error) {
			calledEndpoint = args[1]
			resp, _ := json.Marshal(struct {
				Content  string `json:"content"`
				Encoding string `json:"encoding"`
			}{
				Content:  base64.StdEncoding.EncodeToString([]byte("content")),
				Encoding: "base64",
			})
			return resp, nil
		},
	}

	_, err := r.resolveGitHub("github://owner/repo/file.go@v2.0", "dest.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledEndpoint != "repos/owner/repo/contents/file.go?ref=v2.0" {
		t.Errorf("endpoint: got %q, want %q", calledEndpoint, "repos/owner/repo/contents/file.go?ref=v2.0")
	}
}

func TestResolveGitHub_APIError(t *testing.T) {
	r := &SourceResolver{
		RunGH: func(args ...string) ([]byte, error) {
			return nil, fmt.Errorf("API error: not found")
		},
	}

	_, err := r.resolveGitHub("github://owner/repo/missing.go", "dest.go")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveGitHubDir(t *testing.T) {
	fileContent := "package main"
	encodedContent := base64.StdEncoding.EncodeToString([]byte(fileContent))

	dirResp, _ := json.Marshal([]struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}{
		{Name: "main.go", Path: "src/main.go", Type: "file"},
		{Name: "util.go", Path: "src/util.go", Type: "file"},
	})

	fileResp, _ := json.Marshal(struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}{
		Content:  encodedContent,
		Encoding: "base64",
	})

	r := &SourceResolver{
		RunGH: func(args ...string) ([]byte, error) {
			return fileResp, nil
		},
	}

	entries, err := r.resolveGitHubDir(dirResp, "owner", "repo", "src", "", "dest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Path != "dest/main.go" {
		t.Errorf("entry[0] path: got %q, want %q", entries[0].Path, "dest/main.go")
	}
	if entries[1].Path != "dest/util.go" {
		t.Errorf("entry[1] path: got %q, want %q", entries[1].Path, "dest/util.go")
	}
	if entries[0].Content != fileContent {
		t.Errorf("entry[0] content: got %q, want %q", entries[0].Content, fileContent)
	}
}

func TestResolveGitHubDir_InvalidJSON(t *testing.T) {
	r := &SourceResolver{}
	_, err := r.resolveGitHubDir([]byte("not json"), "o", "r", "p", "", "d")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestResolveLocal_File(t *testing.T) {
	dir := t.TempDir()
	content := "local file content\n"
	if err := os.WriteFile(filepath.Join(dir, "src.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := resolveLocal("src.txt", "dest.txt", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Path != "dest.txt" {
		t.Errorf("path: got %q, want %q", entries[0].Path, "dest.txt")
	}
	if entries[0].Content != content {
		t.Errorf("content: got %q, want %q", entries[0].Content, content)
	}
}

func TestResolveLocal_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	absPath := filepath.Join(dir, "abs.txt")
	content := "absolute content"
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := resolveLocal(absPath, "dest.txt", "/some/other/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries[0].Content != content {
		t.Errorf("content: got %q, want %q", entries[0].Content, content)
	}
}

func TestResolveLocal_Directory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "a.txt"), []byte("aaa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "b.txt"), []byte("bbb"), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := resolveLocal("subdir", "dest", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}
}

func TestResolveLocal_NotFound(t *testing.T) {
	_, err := resolveLocal("nonexistent.txt", "dest.txt", t.TempDir())
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestResolveFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "local.txt"), []byte("local"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &SourceResolver{}

	t.Run("entry with content is passed through", func(t *testing.T) {
		files := []FileEntry{
			{Path: "dest.txt", Content: "inline content"},
		}
		result, err := r.ResolveFiles(files, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if result[0].Content != "inline content" {
			t.Errorf("content: got %q, want %q", result[0].Content, "inline content")
		}
	})

	t.Run("entry without source is passed through", func(t *testing.T) {
		files := []FileEntry{
			{Path: "dest.txt"},
		}
		result, err := r.ResolveFiles(files, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
	})

	t.Run("local source is resolved", func(t *testing.T) {
		files := []FileEntry{
			{Path: "dest.txt", Source: "local.txt"},
		}
		result, err := r.ResolveFiles(files, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if result[0].Content != "local" {
			t.Errorf("content: got %q, want %q", result[0].Content, "local")
		}
	})

	t.Run("github source without RunGH fails", func(t *testing.T) {
		files := []FileEntry{
			{Path: "dest.txt", Source: "github://owner/repo/file.txt"},
		}
		_, err := r.ResolveFiles(files, dir)
		if err == nil {
			t.Fatal("expected error for github source without RunGH")
		}
	})

	t.Run("local source not found fails", func(t *testing.T) {
		files := []FileEntry{
			{Path: "dest.txt", Source: "missing.txt"},
		}
		_, err := r.ResolveFiles(files, dir)
		if err == nil {
			t.Fatal("expected error for missing source")
		}
	})
}
