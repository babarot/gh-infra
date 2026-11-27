package manifest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// Colors that are valid hex but resolve as numbers in YAML when unquoted
// ("5319e7" and "1e1000" as floats, "008672" as an int) must stay strings.
func TestLabel_MarshalYAML_QuotesColor(t *testing.T) {
	desc := "Something isn't working"
	empty := ""
	tests := []struct {
		name  string
		label Label
		want  string
	}{
		{"float-like", Label{Name: "a", Color: "5319e7"}, "name: a\ncolor: \"5319e7\"\n"},
		{"exponent", Label{Name: "a", Color: "1e1000"}, "name: a\ncolor: \"1e1000\"\n"},
		{"int with leading zeros", Label{Name: "a", Color: "008672"}, "name: a\ncolor: \"008672\"\n"},
		{"digits only", Label{Name: "a", Color: "000000"}, "name: a\ncolor: \"000000\"\n"},
		{"letters", Label{Name: "a", Color: "d73a4a"}, "name: a\ncolor: \"d73a4a\"\n"},
		{"description", Label{Name: "bug", Description: &desc, Color: "d73a4a"},
			"name: bug\ndescription: Something isn't working\ncolor: \"d73a4a\"\n"},
		{"number-like name and description", Label{Name: "008672", Description: &[]string{"5319e7"}[0], Color: "d73a4a"},
			"name: \"008672\"\ndescription: \"5319e7\"\ncolor: \"d73a4a\"\n"},
		{"plain text stays plain", Label{Name: "kind/bug", Description: &[]string{"v1.2 is out"}[0], Color: "d73a4a"},
			"name: kind/bug\ndescription: v1.2 is out\ncolor: \"d73a4a\"\n"},
		{"empty description is kept", Label{Name: "a", Description: &empty, Color: "d73a4a"},
			"name: a\ndescription: \"\"\ncolor: \"d73a4a\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := yaml.Marshal(tt.label)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}

			var back Label
			if err := yaml.Unmarshal(got, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back.Color != tt.label.Color || back.Name != tt.label.Name {
				t.Errorf("round trip = %+v, want %+v", back, tt.label)
			}
		})
	}
}

func TestLabel_MarshalYAML_QuotesColorInSpec(t *testing.T) {
	spec := RepositorySpec{Labels: []Label{{Name: "a", Color: "5319e7"}, {Name: "b", Color: "008672"}}}
	got, err := yaml.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`color: "5319e7"`, `color: "008672"`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
}

// Label.MarshalYAML hand-copies the fields; a field added to Label must show up
// in the output too.
func TestLabel_MarshalYAML_KeepsEveryField(t *testing.T) {
	var l Label
	v := reflect.ValueOf(&l).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("x")
		case reflect.Pointer:
			f.Set(reflect.New(f.Type().Elem()))
			if f.Elem().Kind() == reflect.String {
				f.Elem().SetString("x")
			}
		default:
			t.Fatalf("field %s: kind %s is not handled by this test; extend it", v.Type().Field(i).Name, f.Kind())
		}
	}
	got, err := yaml.Marshal(l)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for i := 0; i < v.NumField(); i++ {
		key, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("yaml"), ",")
		if !strings.Contains(string(got), key+":") {
			t.Errorf("field %s (%q) is missing from the marshaled output:\n%s", v.Type().Field(i).Name, key, got)
		}
	}
}
