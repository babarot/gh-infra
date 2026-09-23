package manifest

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestRulesetUpdateUnmarshalBool(t *testing.T) {
	for _, want := range []bool{true, false} {
		input := "update: false"
		if want {
			input = "update: true"
		}
		var out struct {
			Update *RulesetUpdate `yaml:"update"`
		}
		if err := yaml.Unmarshal([]byte(input), &out); err != nil {
			t.Fatalf("unmarshal %q: %v", input, err)
		}
		if out.Update == nil || out.Update.Enabled == nil || *out.Update.Enabled != want {
			t.Fatalf("Enabled = %v, want %t", out.Update, want)
		}
		if out.Update.AllowsFetchAndMerge != nil {
			t.Fatalf("AllowsFetchAndMerge = %v, want nil", *out.Update.AllowsFetchAndMerge)
		}
	}
}

func TestRulesetUpdateUnmarshalObject(t *testing.T) {
	input := `
update:
  allows_fetch_and_merge: true
`
	var out struct {
		Update *RulesetUpdate `yaml:"update"`
	}
	if err := yaml.Unmarshal([]byte(input), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Update == nil || out.Update.Enabled == nil || !*out.Update.Enabled {
		t.Fatal("object form should enable the update rule")
	}
	if out.Update.AllowsFetchAndMerge == nil || !*out.Update.AllowsFetchAndMerge {
		t.Fatal("allows_fetch_and_merge should be true")
	}
}

func TestRulesetUpdateMarshal(t *testing.T) {
	tests := []struct {
		name string
		in   RulesetUpdate
		want string
	}{
		{name: "enabled", in: RulesetUpdate{Enabled: Ptr(true)}, want: "true"},
		{name: "disabled", in: RulesetUpdate{Enabled: Ptr(false)}, want: "false"},
		{
			name: "parameters",
			in: RulesetUpdate{
				Enabled:             Ptr(true),
				AllowsFetchAndMerge: Ptr(true),
			},
			want: "allows_fetch_and_merge: true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(&tt.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got := strings.TrimSpace(string(data)); got != tt.want {
				t.Fatalf("marshal = %q, want %q", got, tt.want)
			}
		})
	}
}
