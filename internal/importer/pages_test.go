package importer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/babarot/gh-infra/internal/manifest"
)

// import --into writes the Pages site GitHub has into the manifest: the
// whole site when the manifest has none, its removal when GitHub has none,
// and otherwise the fields that differ, keeping the rest of the manifest.
func TestDiffRepository_Pages(t *testing.T) {
	ptr := manifest.Ptr[string]
	bptr := manifest.Ptr[bool]
	workflow := &manifest.Pages{BuildType: ptr("workflow"), CNAME: ptr("gomi.dev"), HTTPSEnforced: bptr(false)}
	for _, tt := range []struct {
		name     string
		local    string // the spec's pages block, "" for none
		imported *manifest.Pages
		want     *manifest.Pages // nil: the manifest has no pages after the patch
	}{
		{name: "added", imported: workflow, want: workflow},
		{name: "removed", local: `
  pages:
    build_type: workflow
    cname: gomi.dev
`},
		{name: "domain changed", local: `
  pages:
    build_type: workflow
    cname: old.example.com
    https_enforced: false
`, imported: workflow, want: workflow},
		{name: "moved to workflow", local: `
  pages:
    build_type: legacy
    source:
      branch: main
      path: /docs
    cname: gomi.dev
    https_enforced: false
`, imported: workflow, want: workflow},
		{name: "moved to a branch", local: `
  pages:
    build_type: workflow
    https_enforced: true
`, imported: &manifest.Pages{BuildType: ptr("legacy"), Source: &manifest.PagesSource{Branch: "gh-pages", Path: "/"}, HTTPSEnforced: bptr(true)},
			want: &manifest.Pages{BuildType: ptr("legacy"), Source: &manifest.PagesSource{Branch: "gh-pages", Path: "/"}, HTTPSEnforced: bptr(true)}},
		{name: "source path left out", local: `
  pages:
    build_type: legacy
    source:
      branch: gh-pages
`, imported: &manifest.Pages{BuildType: ptr("legacy"), Source: &manifest.PagesSource{Branch: "gh-pages", Path: "/"}},
			want: &manifest.Pages{BuildType: ptr("legacy"), Source: &manifest.PagesSource{Branch: "gh-pages"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte("apiVersion: gh-infra/v1\nkind: Repository\nmetadata:\n  name: r\n  owner: o\nspec:\n  description: kept\n" + strings.TrimPrefix(tt.local, "\n"))
			var local manifest.Repository
			if err := yaml.Unmarshal(data, &local); err != nil {
				t.Fatal(err)
			}
			imported := manifest.Repository{Spec: manifest.RepositorySpec{Description: ptr("kept"), Pages: tt.imported}}
			mb := map[string][]byte{"r.yaml": data}
			if _, err := DiffRepository(DiffInput{
				Repos:         []*manifest.RepositoryDocument{{Resource: &local, SourcePath: "r.yaml"}},
				Imported:      &imported,
				ManifestBytes: mb,
			}); err != nil {
				t.Fatal(err)
			}
			var got manifest.Repository
			if err := yaml.Unmarshal(mb["r.yaml"], &got); err != nil {
				t.Fatalf("patched manifest does not parse: %v\n%s", err, mb["r.yaml"])
			}
			if !reflect.DeepEqual(got.Spec.Pages, tt.want) {
				t.Errorf("pages = %s, want %s\n%s", pagesString(got.Spec.Pages), pagesString(tt.want), mb["r.yaml"])
			}
			if got.Spec.Description == nil || *got.Spec.Description != "kept" {
				t.Errorf("the rest of the manifest changed:\n%s", mb["r.yaml"])
			}
			if err := got.Validate(); err != nil {
				t.Errorf("patched manifest is invalid: %v", err)
			}
		})
	}
}

func pagesString(p *manifest.Pages) string {
	if p == nil {
		return "<none>"
	}
	return pagesSummary(p)
}

// A RepositorySet entry writes the Pages fields that differ from the
// defaults; a domain the defaults set that GitHub does not have is removed
// with "".
func TestMinimalOverride_Pages(t *testing.T) {
	ptr := manifest.Ptr[string]
	defaults := &manifest.Pages{BuildType: ptr("workflow"), CNAME: ptr("docs.example.com"), HTTPSEnforced: manifest.Ptr(true)}
	for _, tt := range []struct {
		name     string
		imported *manifest.Pages
		want     *manifest.Pages
	}{
		{name: "the same", imported: defaults},
		{name: "none on GitHub", imported: nil},
		{name: "no domain", imported: &manifest.Pages{BuildType: ptr("workflow"), HTTPSEnforced: manifest.Ptr(true)},
			want: &manifest.Pages{CNAME: ptr("")}},
		{name: "from a branch", imported: &manifest.Pages{BuildType: ptr("legacy"), Source: &manifest.PagesSource{Branch: "gh-pages", Path: "/"}, CNAME: ptr("docs.example.com"), HTTPSEnforced: manifest.Ptr(true)},
			want: &manifest.Pages{BuildType: ptr("legacy"), Source: &manifest.PagesSource{Branch: "gh-pages", Path: "/"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := minimalPages(defaults, tt.imported)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("override = %s, want %s", pagesString(got), pagesString(tt.want))
			}
			// The entry merged over the defaults is what GitHub has.
			if tt.imported != nil {
				spec := manifest.RepositorySpec{Pages: defaults}
				if got != nil {
					merged := *defaults
					if got.BuildType != nil {
						merged.BuildType = got.BuildType
					}
					if got.Source != nil {
						merged.Source = got.Source
					}
					if got.CNAME != nil {
						merged.CNAME = got.CNAME
					}
					spec.Pages = &merged
				}
				if len(comparePages(spec.Pages, tt.imported)) > 0 {
					t.Errorf("entry over the defaults %s differs from GitHub %s", pagesString(spec.Pages), pagesString(tt.imported))
				}
			}
		})
	}
	if w := missingDefaultsWarnings(manifest.RepositorySpec{Pages: defaults}, manifest.RepositorySpec{}); len(w) != 1 || w[0].Field != "pages" {
		t.Errorf("warnings = %+v, want one for pages", w)
	}
}
