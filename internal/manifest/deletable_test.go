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
		if n.Get() != nil {
			t.Errorf("value should be nil, got %v", n.Get())
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
		if got := n.Get(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("value should be [a b], got %v", got)
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
		if got := n.Get(); len(got) != 1 || got[0] != "a" {
			t.Errorf("value should be [a], got %v", got)
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
	if len(n.Get()) != 3 {
		t.Errorf("expected 3 items, got %d", len(n.Get()))
	}

	null := DeleteValue[[]int]()
	if !null.IsSet() {
		t.Error("DeleteValue should set isSet")
	}
	if !null.IsDelete() {
		t.Error("DeleteValue should be null")
	}
}

func TestDeletable_Accessors(t *testing.T) {
	var unset Deletable[[]string]
	if unset.HasValue() {
		t.Error("unset Deletable should not have a value")
	}
	if _, ok := unset.GetOK(); ok {
		t.Error("unset Deletable GetOK should be false")
	}
	if HasItems(unset) {
		t.Error("unset Deletable should not have items")
	}

	set := NewDeletable([]string{"a", "b"})
	if !set.HasValue() {
		t.Error("set Deletable should have a value")
	}
	if got := set.Get(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Get() = %v, want [a b]", got)
	}
	if got, ok := set.GetOK(); !ok || len(got) != 2 {
		t.Errorf("GetOK() = %v, %v; want two values and true", got, ok)
	}
	if !HasItems(set) {
		t.Error("set Deletable should have items")
	}

	empty := NewDeletable([]string{})
	if !empty.HasValue() {
		t.Error("empty Deletable should still have a concrete value")
	}
	if HasItems(empty) {
		t.Error("empty Deletable should not have items")
	}

	deleted := DeleteValue[[]string]()
	if deleted.HasValue() {
		t.Error("deleted Deletable should not have a concrete value")
	}
	if _, ok := deleted.GetOK(); ok {
		t.Error("deleted Deletable GetOK should be false")
	}
	if HasItems(deleted) {
		t.Error("deleted Deletable should not have items")
	}
}

func TestMergeDeletableSlice(t *testing.T) {
	merge := func(base, override []string) []string {
		return append(append([]string{}, base...), override...)
	}

	t.Run("delete override wins", func(t *testing.T) {
		got := MergeDeletableSlice(NewDeletable([]string{"base"}), DeleteValue[[]string](), merge)
		if !got.IsDelete() {
			t.Fatal("expected delete marker")
		}
	})

	t.Run("non-empty override merges", func(t *testing.T) {
		got := MergeDeletableSlice(NewDeletable([]string{"base"}), NewDeletable([]string{"override"}), merge)
		if got.IsDelete() {
			t.Fatal("did not expect delete marker")
		}
		if items := got.Get(); len(items) != 2 || items[0] != "base" || items[1] != "override" {
			t.Fatalf("items = %v, want [base override]", items)
		}
	})

	t.Run("empty override leaves base", func(t *testing.T) {
		got := MergeDeletableSlice(NewDeletable([]string{"base"}), NewDeletable([]string{}), merge)
		if items := got.Get(); len(items) != 1 || items[0] != "base" {
			t.Fatalf("items = %v, want [base]", items)
		}
	})

	t.Run("omitted override leaves base", func(t *testing.T) {
		got := MergeDeletableSlice(NewDeletable([]string{"base"}), Deletable[[]string]{}, merge)
		if items := got.Get(); len(items) != 1 || items[0] != "base" {
			t.Fatalf("items = %v, want [base]", items)
		}
	})
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
		if spec.Items.Get() != nil {
			t.Errorf("expected delete marker to clear value, got %v", spec.Items.Get())
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
		if got := spec.Items.Get(); len(got) != 1 || got[0] != "keep" {
			t.Errorf("expected existing value to remain, got %v", got)
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
		if got := spec.Items.Get(); len(got) != 1 || got[0] != "decoded" {
			t.Errorf("expected decoded value to remain, got %v", got)
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
