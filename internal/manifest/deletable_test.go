package manifest

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestDeletable_UnmarshalYAML(t *testing.T) {
	t.Run("null value", func(t *testing.T) {
		var n Deletable[[]string]
		if err := n.UnmarshalYAML([]byte("null")); err != nil {
			t.Fatal(err)
		}
		if !n.IsSet() {
			t.Error("IsSet should be true")
		}
		if !n.IsDelete() {
			t.Error("IsDelete should be true")
		}
		if n.Value != nil {
			t.Errorf("Value should be nil, got %v", n.Value)
		}
	})

	t.Run("tilde null", func(t *testing.T) {
		var n Deletable[[]string]
		if err := n.UnmarshalYAML([]byte("~")); err != nil {
			t.Fatal(err)
		}
		if !n.IsSet() {
			t.Error("IsSet should be true")
		}
		if !n.IsDelete() {
			t.Error("IsDelete should be true")
		}
	})

	t.Run("populated value", func(t *testing.T) {
		var n Deletable[[]string]
		if err := n.UnmarshalYAML([]byte("- a\n- b")); err != nil {
			t.Fatal(err)
		}
		if !n.IsSet() {
			t.Error("IsSet should be true")
		}
		if n.IsDelete() {
			t.Error("IsDelete should be false")
		}
		if len(n.Value) != 2 || n.Value[0] != "a" || n.Value[1] != "b" {
			t.Errorf("Value should be [a b], got %v", n.Value)
		}
	})

	t.Run("reused value clears delete marker", func(t *testing.T) {
		n := DeleteValue[[]string]()
		if err := n.UnmarshalYAML([]byte("- a")); err != nil {
			t.Fatal(err)
		}
		if n.IsDelete() {
			t.Error("IsDelete should be false after decoding a concrete value")
		}
		if len(n.Value) != 1 || n.Value[0] != "a" {
			t.Errorf("Value should be [a], got %v", n.Value)
		}
	})

	t.Run("zero value (not called)", func(t *testing.T) {
		var n Deletable[[]string]
		// UnmarshalYAML not called — simulates field omission
		if n.IsSet() {
			t.Error("IsSet should be false")
		}
		if n.IsDelete() {
			t.Error("IsDelete should be false")
		}
	})
}

func TestDeletable_IsZero(t *testing.T) {
	var unset Deletable[[]string]
	if !unset.IsZero() {
		t.Error("unset Deletable should be zero")
	}

	set := NewDeletable([]string{"a"})
	if set.IsZero() {
		t.Error("set Deletable should not be zero")
	}

	null := DeleteValue[[]string]()
	if null.IsZero() {
		t.Error("null Deletable should not be zero (it is explicitly set)")
	}
}

func TestDeletable_Constructors(t *testing.T) {
	n := NewDeletable([]int{1, 2, 3})
	if !n.IsSet() {
		t.Error("NewDeletable should set isSet")
	}
	if n.IsDelete() {
		t.Error("NewDeletable should not be null")
	}
	if len(n.Value) != 3 {
		t.Errorf("expected 3 items, got %d", len(n.Value))
	}

	null := DeleteValue[[]int]()
	if !null.IsSet() {
		t.Error("DeleteValue should set isSet")
	}
	if !null.IsDelete() {
		t.Error("DeleteValue should be null")
	}
}

func TestDeletable_MarshalYAML(t *testing.T) {
	t.Run("null produces nil", func(t *testing.T) {
		n := DeleteValue[[]string]()
		v, err := n.MarshalYAML()
		if err != nil {
			t.Fatal(err)
		}
		if v != nil {
			t.Errorf("expected nil, got %v", v)
		}
	})

	t.Run("value produces inner value", func(t *testing.T) {
		n := NewDeletable([]string{"a", "b"})
		v, err := n.MarshalYAML()
		if err != nil {
			t.Fatal(err)
		}
		items, ok := v.([]string)
		if !ok {
			t.Fatalf("expected []string, got %T", v)
		}
		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}
	})

	t.Run("round trip with struct", func(t *testing.T) {
		type Spec struct {
			Items Deletable[[]string] `yaml:"items,omitempty"`
			Name  string              `yaml:"name"`
		}

		// Populated value
		s := Spec{
			Items: NewDeletable([]string{"x", "y"}),
			Name:  "test",
		}
		data, err := yaml.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		out := string(data)
		if out == "" {
			t.Fatal("empty output")
		}

		// Unset value (should be omitted)
		s2 := Spec{Name: "test2"}
		data2, err := yaml.Marshal(s2)
		if err != nil {
			t.Fatal(err)
		}
		out2 := string(data2)
		if strings.Contains(out2, "items") {
			t.Errorf("unset Deletable should be omitted, got: %s", out2)
		}
	})
}

func TestApplyDeletableMarkers(t *testing.T) {
	t.Run("marks delete when raw yaml has null", func(t *testing.T) {
		type Spec struct {
			Items Deletable[[]string] `yaml:"items,omitempty"`
			Name  string              `yaml:"name,omitempty"`
		}
		spec := Spec{Items: NewDeletable([]string{"old"})}
		raw := map[string]any{"items": nil}

		if err := applyDeletableMarkers(raw, &spec); err != nil {
			t.Fatal(err)
		}
		if !spec.Items.IsDelete() {
			t.Fatal("expected Items to be marked for delete")
		}
		if spec.Items.Value != nil {
			t.Errorf("expected delete marker to clear Value, got %v", spec.Items.Value)
		}
	})

	t.Run("omitted raw key leaves field unchanged", func(t *testing.T) {
		type Spec struct {
			Items Deletable[[]string] `yaml:"items,omitempty"`
		}
		spec := Spec{Items: NewDeletable([]string{"keep"})}

		if err := applyDeletableMarkers(map[string]any{}, &spec); err != nil {
			t.Fatal(err)
		}
		if spec.Items.IsDelete() {
			t.Fatal("expected Items not to be marked for delete")
		}
		if len(spec.Items.Value) != 1 || spec.Items.Value[0] != "keep" {
			t.Errorf("expected existing value to remain, got %v", spec.Items.Value)
		}
	})

	t.Run("non-null raw key leaves decoded value intact", func(t *testing.T) {
		type Spec struct {
			Items Deletable[[]string] `yaml:"items,omitempty"`
		}
		spec := Spec{Items: NewDeletable([]string{"decoded"})}
		raw := map[string]any{"items": []any{"decoded"}}

		if err := applyDeletableMarkers(raw, &spec); err != nil {
			t.Fatal(err)
		}
		if spec.Items.IsDelete() {
			t.Fatal("expected Items not to be marked for delete")
		}
		if len(spec.Items.Value) != 1 || spec.Items.Value[0] != "decoded" {
			t.Errorf("expected decoded value to remain, got %v", spec.Items.Value)
		}
	})

	t.Run("deletable field must have yaml key", func(t *testing.T) {
		type Spec struct {
			Items Deletable[[]string] `yaml:"-"`
		}
		err := applyDeletableMarkers(map[string]any{"items": nil}, &Spec{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "Deletable field must have a yaml key") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
