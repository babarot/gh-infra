package manifest

import (
	"strings"
	"testing"
)

func TestValidateStruct_Required(t *testing.T) {
	type S struct {
		Name string `yaml:"name" validate:"required"`
	}
	if err := ValidateStruct("", &S{Name: "ok"}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	err := ValidateStruct("", &S{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty required field")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateStruct_RequiredWithPrefix(t *testing.T) {
	type Metadata struct {
		Name string `yaml:"name" validate:"required"`
	}
	type Top struct {
		Metadata Metadata `yaml:"metadata"`
	}
	err := ValidateStruct("", &Top{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "metadata.name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateStruct_RequiredWithContextPrefix(t *testing.T) {
	type S struct {
		Name string `yaml:"name" validate:"required"`
	}
	err := ValidateStruct("my-repo", &S{Name: ""})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "my-repo: name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateStruct_RequiredSlice(t *testing.T) {
	type S struct {
		Items []string `yaml:"items" validate:"required"`
	}
	if err := ValidateStruct("", &S{Items: []string{"a"}}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	err := ValidateStruct("", &S{Items: nil})
	if err == nil {
		t.Fatal("expected error for nil required slice")
	}
	if !strings.Contains(err.Error(), "items is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateStruct_OneOf(t *testing.T) {
	type S struct {
		Color string `yaml:"color" validate:"oneof=red green blue"`
	}
	if err := ValidateStruct("", &S{Color: "red"}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	err := ValidateStruct("", &S{Color: "yellow"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid color") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "must be one of: red, green, blue") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateStruct_OmitemptyOneof(t *testing.T) {
	type S struct {
		Mode *string `yaml:"mode" validate:"omitempty,oneof=fast slow"`
	}
	// nil pointer → skip
	if err := ValidateStruct("", &S{Mode: nil}); err != nil {
		t.Errorf("expected no error for nil, got: %v", err)
	}
	// valid value
	fast := "fast"
	if err := ValidateStruct("", &S{Mode: &fast}); err != nil {
		t.Errorf("expected no error for valid value, got: %v", err)
	}
	// invalid value
	bad := "turbo"
	err := ValidateStruct("", &S{Mode: &bad})
	if err == nil {
		t.Fatal("expected error for invalid value")
	}
	if !strings.Contains(err.Error(), "invalid mode") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateStruct_OmitemptyString(t *testing.T) {
	type S struct {
		Via string `yaml:"via" validate:"omitempty,oneof=push pull_request"`
	}
	// empty string → skip
	if err := ValidateStruct("", &S{Via: ""}); err != nil {
		t.Errorf("expected no error for empty, got: %v", err)
	}
	// valid
	if err := ValidateStruct("", &S{Via: "push"}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	// invalid
	err := ValidateStruct("", &S{Via: "deploy"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid via") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrateDeprecated_Basic(t *testing.T) {
	type S struct {
		Via            string   `yaml:"via"`
		OldCommit      string   `yaml:"commit_strategy" deprecated:"via:use 'via' instead"`
		DeprecWarnings []string `yaml:"-"`
	}
	s := S{OldCommit: "push"}
	warnings, err := MigrateDeprecated(&s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Via != "push" {
		t.Errorf("expected Via to be migrated to 'push', got %q", s.Via)
	}
	if s.OldCommit != "" {
		t.Errorf("expected OldCommit to be cleared, got %q", s.OldCommit)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "deprecated") {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestMigrateDeprecated_Conflict(t *testing.T) {
	type S struct {
		Via       string `yaml:"via"`
		OldCommit string `yaml:"commit_strategy" deprecated:"via:use 'via' instead"`
	}
	s := S{Via: "push", OldCommit: "pull_request"}
	_, err := MigrateDeprecated(&s)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "cannot specify both") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrateDeprecated_NoTarget(t *testing.T) {
	type S struct {
		OnDrift string `yaml:"on_drift" deprecated:":will be ignored"`
	}
	s := S{OnDrift: "warn"}
	warnings, err := MigrateDeprecated(&s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.OnDrift != "" {
		t.Errorf("expected OnDrift to be cleared, got %q", s.OnDrift)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestMigrateDeprecated_EmptyField(t *testing.T) {
	type S struct {
		Via       string `yaml:"via"`
		OldCommit string `yaml:"commit_strategy" deprecated:"via:use 'via' instead"`
	}
	s := S{Via: "push", OldCommit: ""}
	warnings, err := MigrateDeprecated(&s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for empty deprecated field, got %d", len(warnings))
	}
}

func TestValidateStruct_Unique(t *testing.T) {
	type Item struct {
		Name string `yaml:"name"`
	}
	type S struct {
		Items []Item `yaml:"items" validate:"unique=name"`
	}
	// No duplicates
	if err := ValidateStruct("", &S{Items: []Item{{Name: "a"}, {Name: "b"}}}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	// Duplicates
	err := ValidateStruct("", &S{Items: []Item{{Name: "a"}, {Name: "a"}}})
	if err == nil {
		t.Fatal("expected error for duplicate")
	}
	if !strings.Contains(err.Error(), "duplicate name") {
		t.Errorf("unexpected error: %v", err)
	}
	// Empty slice is fine
	if err := ValidateStruct("", &S{}); err != nil {
		t.Errorf("expected no error for empty slice, got: %v", err)
	}
}

func TestValidateStruct_UniqueUnknownField(t *testing.T) {
	type Item struct {
		Name string `yaml:"name"`
	}
	type S struct {
		// The Go field name instead of the yaml one
		Items []Item `yaml:"items" validate:"unique=Name"`
	}
	err := ValidateStruct("", &S{Items: []Item{{Name: "a"}}})
	if err == nil {
		t.Fatal("expected an error for a unique key that names no yaml field")
	}
	if !strings.Contains(err.Error(), "unique=Name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateStruct_Exclusive(t *testing.T) {
	type S struct {
		Content string `yaml:"content" validate:"exclusive=source"`
		Source  string `yaml:"source"`
	}
	// Neither set
	if err := ValidateStruct("", &S{}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	// Only content
	if err := ValidateStruct("", &S{Content: "x"}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	// Only source
	if err := ValidateStruct("", &S{Source: "y"}); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	// Both set
	err := ValidateStruct("", &S{Content: "x", Source: "y"})
	if err == nil {
		t.Fatal("expected error for exclusive fields")
	}
	if !strings.Contains(err.Error(), "cannot have both content and source") {
		t.Errorf("unexpected error: %v", err)
	}
}
