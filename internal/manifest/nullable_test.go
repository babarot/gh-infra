package manifest

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestNullable_UnmarshalYAML(t *testing.T) {
	t.Run("null value", func(t *testing.T) {
		var n Nullable[[]string]
		if err := n.UnmarshalYAML([]byte("null")); err != nil {
			t.Fatal(err)
		}
		if !n.IsSet() {
			t.Error("IsSet should be true")
		}
		if !n.IsNull() {
			t.Error("IsNull should be true")
		}
		if n.Value != nil {
			t.Errorf("Value should be nil, got %v", n.Value)
		}
	})

	t.Run("tilde null", func(t *testing.T) {
		var n Nullable[[]string]
		if err := n.UnmarshalYAML([]byte("~")); err != nil {
			t.Fatal(err)
		}
		if !n.IsSet() {
			t.Error("IsSet should be true")
		}
		if !n.IsNull() {
			t.Error("IsNull should be true")
		}
	})

	t.Run("populated value", func(t *testing.T) {
		var n Nullable[[]string]
		if err := n.UnmarshalYAML([]byte("- a\n- b")); err != nil {
			t.Fatal(err)
		}
		if !n.IsSet() {
			t.Error("IsSet should be true")
		}
		if n.IsNull() {
			t.Error("IsNull should be false")
		}
		if len(n.Value) != 2 || n.Value[0] != "a" || n.Value[1] != "b" {
			t.Errorf("Value should be [a b], got %v", n.Value)
		}
	})

	t.Run("zero value (not called)", func(t *testing.T) {
		var n Nullable[[]string]
		// UnmarshalYAML not called — simulates field omission
		if n.IsSet() {
			t.Error("IsSet should be false")
		}
		if n.IsNull() {
			t.Error("IsNull should be false")
		}
	})
}

func TestNullable_IsZero(t *testing.T) {
	var unset Nullable[[]string]
	if !unset.IsZero() {
		t.Error("unset Nullable should be zero")
	}

	set := NewNullable([]string{"a"})
	if set.IsZero() {
		t.Error("set Nullable should not be zero")
	}

	null := NullValue[[]string]()
	if null.IsZero() {
		t.Error("null Nullable should not be zero (it is explicitly set)")
	}
}

func TestNullable_Constructors(t *testing.T) {
	n := NewNullable([]int{1, 2, 3})
	if !n.IsSet() {
		t.Error("NewNullable should set isSet")
	}
	if n.IsNull() {
		t.Error("NewNullable should not be null")
	}
	if len(n.Value) != 3 {
		t.Errorf("expected 3 items, got %d", len(n.Value))
	}

	null := NullValue[[]int]()
	if !null.IsSet() {
		t.Error("NullValue should set isSet")
	}
	if !null.IsNull() {
		t.Error("NullValue should be null")
	}
}

func TestNullable_MarshalYAML(t *testing.T) {
	t.Run("null produces nil", func(t *testing.T) {
		n := NullValue[[]string]()
		v, err := n.MarshalYAML()
		if err != nil {
			t.Fatal(err)
		}
		if v != nil {
			t.Errorf("expected nil, got %v", v)
		}
	})

	t.Run("value produces inner value", func(t *testing.T) {
		n := NewNullable([]string{"a", "b"})
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
			Items Nullable[[]string] `yaml:"items,omitempty"`
			Name  string             `yaml:"name"`
		}

		// Populated value
		s := Spec{
			Items: NewNullable([]string{"x", "y"}),
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
			t.Errorf("unset Nullable should be omitted, got: %s", out2)
		}
	})
}

