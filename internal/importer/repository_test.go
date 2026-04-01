package importer

import (
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

func ptr[T any](v T) *T { return &v }

func TestPlanRepository_NoDiff(t *testing.T) {
	local := manifest.RepositorySpec{
		Description: ptr("my repo"),
		Visibility:  ptr("public"),
		Topics:      []string{"go", "cli"},
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Description: ptr("my repo"),
			Visibility:  ptr("public"),
			Topics:      []string{"cli", "go"}, // same set, different order
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  description: my repo
  visibility: public
  topics:
    - go
    - cli
`)

	rp, err := PlanRepository(RepoPlanInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: map[string][]byte{"/tmp/test.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}
	if rp.HasChanges() {
		t.Errorf("expected no changes, got %d diffs: %+v", len(rp.Diffs), rp.Diffs)
	}
}

func TestPlanRepository_ScalarDiff(t *testing.T) {
	local := manifest.RepositorySpec{
		Description: ptr("old desc"),
		Visibility:  ptr("private"),
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Description: ptr("new desc"),
			Visibility:  ptr("public"),
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  description: old desc
  visibility: private
`)

	rp, err := PlanRepository(RepoPlanInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: map[string][]byte{"/tmp/test.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}
	if !rp.HasChanges() {
		t.Fatal("expected changes")
	}

	found := make(map[string]bool)
	for _, d := range rp.Diffs {
		found[d.Field] = true
	}
	if !found["description"] {
		t.Error("expected diff for description")
	}
	if !found["visibility"] {
		t.Error("expected diff for visibility")
	}
}

func TestPlanRepository_TopicsDiff(t *testing.T) {
	local := manifest.RepositorySpec{
		Topics: []string{"go"},
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Topics: []string{"go", "cli"},
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  topics:
    - go
`)

	rp, err := PlanRepository(RepoPlanInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: map[string][]byte{"/tmp/test.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}

	found := false
	for _, d := range rp.Diffs {
		if d.Field == "topics" {
			found = true
		}
	}
	if !found {
		t.Error("expected diff for topics")
	}
}

func TestPlanRepository_FeaturesDiff(t *testing.T) {
	local := manifest.RepositorySpec{
		Features: &manifest.Features{
			Issues: ptr(true),
			Wiki:   ptr(true),
		},
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Features: &manifest.Features{
				Issues: ptr(true),
				Wiki:   ptr(false), // changed
			},
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  features:
    issues: true
    wiki: true
`)

	rp, err := PlanRepository(RepoPlanInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: map[string][]byte{"/tmp/test.yaml": yamlData},
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}

	found := false
	for _, d := range rp.Diffs {
		if d.Field == "features.wiki" {
			found = true
		}
	}
	if !found {
		t.Error("expected diff for features.wiki")
	}

	// features.issues should NOT have a diff (unchanged)
	for _, d := range rp.Diffs {
		if d.Field == "features.issues" {
			t.Error("features.issues should not have a diff (unchanged)")
		}
	}
}

func TestPlanRepository_ManifestBytesUpdated(t *testing.T) {
	local := manifest.RepositorySpec{
		Description: ptr("old"),
	}
	imported := manifest.Repository{
		Spec: manifest.RepositorySpec{
			Description: ptr("new"),
		},
	}

	doc := &manifest.RepositoryDocument{
		Resource:   &manifest.Repository{Spec: local},
		SourcePath: "/tmp/test.yaml",
		DocIndex:   0,
	}

	yamlData := []byte(`apiVersion: gh-infra/v1
kind: Repository
metadata:
  name: test
  owner: org
spec:
  description: old
`)

	mb := map[string][]byte{"/tmp/test.yaml": yamlData}
	_, err := PlanRepository(RepoPlanInput{
		Repos:         []*manifest.RepositoryDocument{doc},
		Imported:      &imported,
		ManifestBytes: mb,
	})
	if err != nil {
		t.Fatalf("PlanRepository error: %v", err)
	}

	// manifestBytes should be updated in-place
	updated := string(mb["/tmp/test.yaml"])
	if updated == string(yamlData) {
		t.Error("manifestBytes should have been updated")
	}
}

func TestMinimalOverride_AllSameAsDefaults(t *testing.T) {
	defaults := manifest.RepositorySpec{
		Visibility: ptr("private"),
		Features: &manifest.Features{
			Issues: ptr(true),
		},
	}

	// imported matches defaults exactly
	imported := manifest.RepositorySpec{
		Visibility: ptr("private"),
		Features: &manifest.Features{
			Issues: ptr(true),
		},
	}

	override := minimalOverride(defaults, imported)

	if override.Visibility != nil {
		t.Errorf("Visibility should be nil (same as defaults), got %v", *override.Visibility)
	}
	if override.Features != nil {
		t.Errorf("Features should be nil (same as defaults), got %+v", override.Features)
	}
}

func TestMinimalOverride_ScalarOverride(t *testing.T) {
	defaults := manifest.RepositorySpec{
		Visibility: ptr("private"),
	}

	imported := manifest.RepositorySpec{
		Visibility: ptr("public"),
	}

	override := minimalOverride(defaults, imported)

	if override.Visibility == nil || *override.Visibility != "public" {
		t.Errorf("Visibility should be 'public', got %v", override.Visibility)
	}
}

func TestMinimalOverride_FeaturePartialOverride(t *testing.T) {
	defaults := manifest.RepositorySpec{
		Features: &manifest.Features{
			Issues: ptr(true),
			Wiki:   ptr(true),
		},
	}

	imported := manifest.RepositorySpec{
		Features: &manifest.Features{
			Issues: ptr(true),  // same
			Wiki:   ptr(false), // different
		},
	}

	override := minimalOverride(defaults, imported)

	if override.Features == nil {
		t.Fatal("Features should not be nil")
	}
	if override.Features.Issues != nil {
		t.Error("Issues should be nil (same as defaults)")
	}
	if override.Features.Wiki == nil || *override.Features.Wiki != false {
		t.Error("Wiki should be false (different from defaults)")
	}
}

func TestMinimalOverride_TopicsOverride(t *testing.T) {
	defaults := manifest.RepositorySpec{
		Topics: []string{"go"},
	}

	imported := manifest.RepositorySpec{
		Topics: []string{"go", "cli"},
	}

	override := minimalOverride(defaults, imported)

	if len(override.Topics) != 2 {
		t.Errorf("Topics should have 2 items (different from defaults), got %d", len(override.Topics))
	}
}

func TestCompareSpecs_NoDiff(t *testing.T) {
	spec := manifest.RepositorySpec{
		Description: ptr("test"),
		Visibility:  ptr("public"),
	}

	diffs := compareSpecs(spec, spec)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d: %+v", len(diffs), diffs)
	}
}
