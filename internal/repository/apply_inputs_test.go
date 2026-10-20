package repository

import (
	"reflect"
	"testing"

	"github.com/babarot/gh-infra/internal/manifest"
)

func rulesetSpec(team string, description string) manifest.RepositorySpec {
	return manifest.RepositorySpec{
		Description: &description,
		Rulesets: []manifest.Ruleset{{
			Name:         "main",
			BypassActors: []manifest.RulesetBypassActor{{Team: team, BypassMode: "always"}},
		}},
		Secrets: []manifest.Secret{{Name: "TOKEN", Value: "s3cret-" + team}},
	}
}

func TestApplyInputs(t *testing.T) {
	createRuleset := []Change{{
		Type: ChangeCreate, Resource: "Ruleset[main]", Field: "ruleset", NewValue: "main",
		Details: []Change{
			{Type: ChangeCreate, Field: "enforcement", NewValue: "active"},
			{Type: ChangeCreate, Field: "bypass_actors", NewValue: "1 actors"},
		},
	}}
	deleteRuleset := func(id int) []Change {
		return []Change{{Type: ChangeDelete, Resource: "Ruleset[main]", Field: "ruleset", OldValue: id,
			Details: []Change{{Type: ChangeDelete, Field: "bypass_actors", OldValue: "1 actors"}}}}
	}
	setSecret := []Change{{Type: ChangeCreate, Resource: manifest.ResourceSecret, Field: "TOKEN", NewValue: "(new)"}}
	updateDescription := []Change{{Type: ChangeUpdate, Resource: manifest.ResourceRepository, Field: "description", OldValue: "a", NewValue: "b"}}

	tests := []struct {
		name         string
		a, b         manifest.RepositorySpec
		changesA     []Change
		changesB     []Change
		wantSameness bool
	}{
		{"bypass actors the plan shows as a count differ",
			rulesetSpec("platform", "x"), rulesetSpec("security", "x"), createRuleset, createRuleset, false},
		{"the same ruleset reads the same",
			rulesetSpec("platform", "x"), rulesetSpec("platform", "y"), createRuleset, createRuleset, true},
		{"a secret value the plan does not show differs",
			rulesetSpec("platform", "x"), rulesetSpec("security", "x"), setSecret, setSecret, false},
		{"fields no change touches are left out",
			rulesetSpec("platform", "x"), rulesetSpec("security", "y"), updateDescription, updateDescription, true},
		{"deleting the same ruleset under different IDs reads the same",
			rulesetSpec("platform", "x"), rulesetSpec("security", "x"), deleteRuleset(1), deleteRuleset(2), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := ApplyInputs(tt.a, tt.changesA), ApplyInputs(tt.b, tt.changesB)
			if same := reflect.DeepEqual(a, b); same != tt.wantSameness {
				t.Errorf("same = %v, want %v\na: %v\nb: %v", same, tt.wantSameness, a, b)
			}
		})
	}
}
