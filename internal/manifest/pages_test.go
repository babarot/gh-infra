package manifest

import (
	"strings"
	"testing"
)

func TestValidatePages(t *testing.T) {
	for _, tt := range []struct {
		name  string
		pages *Pages
		want  string // "" for valid
	}{
		{name: "workflow", pages: &Pages{BuildType: Ptr("workflow"), CNAME: Ptr("docs.example.com"), HTTPSEnforced: Ptr(true)}},
		{name: "legacy", pages: &Pages{BuildType: Ptr("legacy"), Source: &PagesSource{Branch: "gh-pages", Path: "/docs"}}},
		{name: "domain removed", pages: &Pages{CNAME: Ptr("")}},
		{name: "unknown build type", pages: &Pages{BuildType: Ptr("jekyll")}, want: "build_type"},
		{name: "legacy without a source", pages: &Pages{BuildType: Ptr("legacy")}, want: "pages.source is required"},
		{name: "a source with workflow", pages: &Pages{BuildType: Ptr("workflow"), Source: &PagesSource{Branch: "main"}}, want: "build_type: legacy"},
		{name: "a source without a build type", pages: &Pages{Source: &PagesSource{Branch: "main"}}, want: "build_type: legacy"},
		{name: "a source without a branch", pages: &Pages{BuildType: Ptr("legacy"), Source: &PagesSource{Path: "/"}}, want: "branch"},
		{name: "a source path GitHub does not take", pages: &Pages{BuildType: Ptr("legacy"), Source: &PagesSource{Branch: "main", Path: "/site"}}, want: "path"},
		{name: "an uppercase domain", pages: &Pages{CNAME: Ptr("Docs.example.com")}, want: "lowercase domain"},
		{name: "a domain with a scheme", pages: &Pages{CNAME: Ptr("https://docs.example.com")}, want: "lowercase domain"},
		{name: "a domain with a trailing dot", pages: &Pages{CNAME: Ptr("docs.example.com.")}, want: "lowercase domain"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &Repository{Metadata: RepositoryMetadata{Owner: "o", Name: "r"}, Spec: RepositorySpec{Pages: tt.pages}}
			err := r.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

// An entry's pages fields override the defaults' one by one; a source
// replaces the defaults' as a whole.
func TestMergeSpecs_Pages(t *testing.T) {
	defaults := &RepositorySetDefaults{Spec: RepositorySpec{Pages: &Pages{
		BuildType: Ptr("legacy"), Source: &PagesSource{Branch: "gh-pages", Path: "/docs"}, HTTPSEnforced: Ptr(true),
	}}}
	got := mergeSpecs(defaults, RepositorySpec{Pages: &Pages{Source: &PagesSource{Branch: "main"}, CNAME: Ptr("docs.example.com")}}).Pages
	if *got.BuildType != "legacy" || got.Source.Branch != "main" || got.Source.Path != "" || *got.CNAME != "docs.example.com" || !*got.HTTPSEnforced {
		t.Errorf("merged pages = %+v (source %+v)", got, got.Source)
	}
	if defaults.Spec.Pages.Source.Branch != "gh-pages" || defaults.Spec.Pages.CNAME != nil {
		t.Error("the merge changed the defaults")
	}
	// A workflow site has no source: the defaults' is not inherited.
	if got := mergeSpecs(defaults, RepositorySpec{Pages: &Pages{BuildType: Ptr("workflow")}}).Pages; got.Source != nil || *got.BuildType != "workflow" {
		t.Errorf("a workflow entry inherits the defaults' source: %+v", got.Source)
	}
	if got := mergeSpecs(defaults, RepositorySpec{}).Pages; got != defaults.Spec.Pages {
		t.Error("an entry without pages does not inherit the defaults'")
	}
}
