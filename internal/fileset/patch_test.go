package fileset

import (
	"testing"
)

func TestApplyPatches(t *testing.T) {
	tests := []struct {
		name    string
		content string
		patches []string
		want    string
		wantErr bool
	}{
		{
			name:    "no patches",
			content: "hello\n",
			patches: nil,
			want:    "hello\n",
		},
		{
			name:    "empty patch string",
			content: "hello\n",
			patches: []string{""},
			want:    "hello\n",
		},
		{
			name: "single patch replacing lines",
			content: `[tagpr]
	releaseBranch = main
	vPrefix = true
	versionFile = VERSION
	changelog = true
`,
			patches: []string{
				`--- a/.tagpr
+++ b/.tagpr
@@ -1,5 +1,5 @@
 [tagpr]
 	releaseBranch = main
-	vPrefix = true
-	versionFile = VERSION
+	vPrefix = false
+	versionFile = src/version.ts,deno.json,package.json
 	changelog = true
`,
			},
			want: `[tagpr]
	releaseBranch = main
	vPrefix = false
	versionFile = src/version.ts,deno.json,package.json
	changelog = true
`,
		},
		{
			name:    "patch adding lines",
			content: "line1\nline3\n",
			patches: []string{
				"--- a/f\n+++ b/f\n@@ -1,2 +1,3 @@\n line1\n+line2\n line3\n",
			},
			want: "line1\nline2\nline3\n",
		},
		{
			name:    "patch removing lines",
			content: "line1\nline2\nline3\n",
			patches: []string{
				"--- a/f\n+++ b/f\n@@ -1,3 +1,2 @@\n line1\n-line2\n line3\n",
			},
			want: "line1\nline3\n",
		},
		{
			name:    "multiple patches applied in sequence",
			content: "aaa\nbbb\nccc\n",
			patches: []string{
				"--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n aaa\n-bbb\n+bbb2\n ccc\n",
				"--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n aaa\n bbb2\n-ccc\n+ccc2\n",
			},
			want: "aaa\nbbb2\nccc2\n",
		},
		{
			name:    "non-patch text is silently skipped",
			content: "hello\n",
			patches: []string{"not a valid patch"},
			want:    "hello\n",
		},
		{
			name:    "context mismatch returns error",
			content: "wrong content\n",
			patches: []string{
				"--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-expected line\n+new line\n",
			},
			wantErr: true,
		},
		{
			name:    "second patch fails after first succeeds",
			content: "aaa\nbbb\n",
			patches: []string{
				"--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n aaa\n-bbb\n+ccc\n",
				"--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n-nonexistent\n+xxx\n",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyPatches(tt.content, tt.patches)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ApplyPatches() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ApplyPatches() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
