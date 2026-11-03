package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const prFileSetYAML = `apiVersion: gh-infra/v1
kind: FileSet
metadata:
  owner: org
spec:
  repositories:
    - repo
  files:
    - path: a.txt
      content: a
  via: pull_request
`

// The PR branch check spans path arguments, not just documents in one file.
func TestRunValidate_RejectsSharedPRBranchAcrossPaths(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"ci.yaml", "hooks.yaml"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(prFileSetYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}

	err := runValidate(paths, false)

	if err == nil || !strings.Contains(err.Error(), `from branch "gh-infra/sync-org-repo"`) {
		t.Fatalf("expected shared PR branch error, got %v", err)
	}
}

func TestRunValidate_RejectsBadMessageTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fs.yaml")
	content := prFileSetYAML + "  commit_message: \"sync <% .Repo.Nme %>\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runValidate([]string{path}, false)

	if err == nil || !strings.Contains(err.Error(), "commit_message for org/repo") {
		t.Fatalf("expected a commit_message template error, got %v", err)
	}
}

// .Files and descriptions are checked without planning, against every file
// of the FileSet.
func TestRunValidate_ChecksFilesAndDescriptions(t *testing.T) {
	for _, tt := range []struct {
		name, yaml, want string
	}{
		{"files ok", strings.Replace(prFileSetYAML, "content: a\n", "content: a\n      description: A file\n", 1) +
			"  pr_body: \"<% range .Files %><% .Action %> <% .Path %>: <% .Description %><% end %>\"\n", ""},
		{"files field", prFileSetYAML + "  pr_body: \"<% range .Files %><% .Nope %><% end %>\"\n", "pr_body for org/repo"},
		{"description", strings.Replace(prFileSetYAML, "content: a\n", "content: a\n      description: \"<% .Repo.Nme %>\"\n", 1), "description of a.txt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fs.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			err := runValidate([]string{path}, false)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunValidate_RejectsOverrideMatchingNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fs.yaml")
	content := strings.Replace(prFileSetYAML, "    - repo\n", "    - name: repo\n      overrides:\n        - path: b.txt\n          content: b\n", 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runValidate([]string{path}, false)

	if err == nil || !strings.Contains(err.Error(), "override b.txt for repo matches no entry in files") {
		t.Fatalf("expected an override error, got %v", err)
	}
}
