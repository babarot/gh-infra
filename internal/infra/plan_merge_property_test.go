package infra

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

// genPlan is a generated plan: the changes, the specs apply reads, and, per
// repository, the values the plan body does not show, for the expected
// notes.
type genPlan struct {
	repo   []repository.Change
	files  []fileset.Change
	specs  []*manifest.Repository
	unseen map[string]genUnseen
}

// genUnseen is what the generator chose for a repository that the body does
// not show: the ruleset's bypass team and the secret value (settings), the
// README contents (files), and the FileSet that delivers them (delivery).
type genUnseen struct {
	settings, files string
	delivery        *manifest.FileSet
}

// shape is what a repository's body shows: the generator gives a plan a
// few shapes, so that many repositories read the same.
type shape struct {
	noop, create, wiki, description, labels, ruleset, secret, files, readme bool
	oldDescription, oldColor, newDescription, ci                            string
}

func randomShape(r *rand.Rand) shape {
	pick := func(options ...string) string { return options[r.IntN(len(options))] }
	return shape{
		noop: r.IntN(8) == 0, create: r.IntN(8) == 0,
		wiki: r.IntN(2) == 0, description: r.IntN(3) == 0, labels: r.IntN(2) == 0,
		ruleset: r.IntN(2) == 0, secret: r.IntN(3) == 0, files: r.IntN(2) == 0, readme: r.IntN(2) == 0,
		oldDescription: pick("old", "older"), oldColor: pick("ee0701", "ff0000"),
		newDescription: pick("a", "b"), ci: pick(ciDesired, ciDesired2),
	}
}

// generatePlan makes a plan of up to 12 repositories, each of one of three
// shapes, with what the body does not show (the ruleset's bypass team, the
// secret value, the README contents, the FileSet) chosen per repository.
func generatePlan(r *rand.Rand) genPlan {
	g := genPlan{unseen: make(map[string]genUnseen)}
	fileSets := []*manifest.FileSet{
		{Metadata: manifest.FileSetMetadata{Owner: "acme", Name: "sync"}, Spec: manifest.FileSetSpec{Via: manifest.ViaPullRequest, Branch: "sync/a"}},
		{Metadata: manifest.FileSetMetadata{Owner: "acme", Name: "sync"}, Spec: manifest.FileSetSpec{Via: manifest.ViaPullRequest, Branch: "sync/b"}},
	}
	shapes := []shape{randomShape(r), randomShape(r), randomShape(r)}
	pick := func(options ...string) string { return options[r.IntN(len(options))] }
	for i := range 2 + r.IntN(11) {
		name := fmt.Sprintf("acme/repo-%02d", i)
		spec := &manifest.Repository{Metadata: manifest.RepositoryMetadata{Owner: "acme", Name: fmt.Sprintf("repo-%02d", i)}}
		g.specs = append(g.specs, spec)
		sh := shapes[r.IntN(len(shapes))]
		var u genUnseen

		if sh.noop {
			g.repo = append(g.repo, repository.Change{Type: repository.ChangeNoOp, Name: name})
			continue
		}
		if sh.create {
			g.repo = append(g.repo, createRepo(name),
				repository.Change{Type: repository.ChangeCreate, Resource: manifest.ResourceRepository, Name: name, Field: "description", NewValue: sh.newDescription})
			g.unseen[name] = u
			continue
		}
		if sh.wiki {
			g.repo = append(g.repo, group(name, "features", child("wiki", true, false)))
		}
		if sh.description {
			g.repo = append(g.repo, update(name, "description", sh.oldDescription, sh.newDescription))
		}
		if sh.labels {
			g.repo = append(g.repo, labelChanges(name, sh.oldColor)...)
		}
		if sh.ruleset {
			team := pick("platform", "security")
			u.settings += "team:" + team
			spec.Spec.Rulesets = []manifest.Ruleset{{Name: "main", BypassActors: []manifest.RulesetBypassActor{{Team: team, BypassMode: "always"}}}}
			g.repo = append(g.repo, repository.Change{
				Type: repository.ChangeCreate, Resource: "Ruleset[main]", Name: name, Field: "ruleset", NewValue: "main",
				Details: []repository.Change{{Type: repository.ChangeCreate, Field: "bypass_actors", NewValue: "1 actors"}},
			})
		}
		if sh.secret {
			value := pick("s1", "s2")
			u.settings += " secret:" + value
			spec.Spec.Secrets = []manifest.Secret{{Name: "TOKEN", Value: value}}
			g.repo = append(g.repo, repository.Change{Type: repository.ChangeCreate, Resource: manifest.ResourceSecret, Name: name, Field: "TOKEN", NewValue: "(new)"})
		}
		if sh.files {
			fs := fileSets[r.IntN(len(fileSets))]
			u.delivery = fs
			ci := fileChange(name, "ci.yaml", fileset.ChangeUpdate, ciCurrent, sh.ci, manifest.ViaPullRequest)
			ci.FileSet = fs
			g.files = append(g.files, ci)
			if sh.readme {
				// Same line counts, contents that differ per repository.
				word := pick("api", "web")
				u.files = word
				readme := fileChange(name, "README.md", fileset.ChangeUpdate, "# service\n", "# "+word+" service\n", manifest.ViaPullRequest)
				readme.FileSet = fs
				g.files = append(g.files, readme)
			}
		}
		g.unseen[name] = u
	}
	return g
}

func renderBlocks(blocks []ui.PlanBlock) string {
	var buf bytes.Buffer
	p := ui.NewStandardPrinterWith(&buf, &buf)
	ui.RenderPlanBlocks(p, blocks)
	p.Summary("end")
	return buf.String()
}

func changesOf(name string, g genPlan) ([]repository.Change, []fileset.Change) {
	var rc []repository.Change
	for _, c := range g.repo {
		if c.Name == name && c.Type != repository.ChangeNoOp {
			rc = append(rc, c)
		}
	}
	var fc []fileset.Change
	for _, c := range g.files {
		if c.Target == name {
			fc = append(fc, c)
		}
	}
	return rc, fc
}

// TestPlanMergeProperties checks, over generated plans, that merging
// repositories never changes what the body says about any of them.
func TestPlanMergeProperties(t *testing.T) {
	noted := make(map[ui.UnseenPart]int)
	for seed := range uint64(300) {
		g := generatePlan(rand.New(rand.NewPCG(seed, seed)))
		blocks := planBlocks(g.repo, g.files, nil, unseenOf(g.specs), false)
		byName := make(map[string]ui.PlanBlock, len(blocks))
		for _, b := range blocks {
			byName[b.Names[0]] = b
		}
		merged := ui.RewritePlanBlocks(blocks)

		// (b) every repository with changes is in exactly one block.
		var got []string
		for _, b := range merged {
			got = append(got, b.Names...)
		}
		want := plannedRepos(g.repo, g.files)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("seed %d: (b) repositories in blocks = %v, want each of %v once", seed, got, want)
		}

		for _, mb := range merged {
			for _, name := range mb.Names {
				// (a) the merged body is what the repository shows alone.
				one := mb
				one.Names, one.Differ = []string{name}, nil
				alone := ui.RewritePlanBlocks([]ui.PlanBlock{byName[name]})
				if a, b := renderBlocks([]ui.PlanBlock{one}), renderBlocks(alone); a != b {
					t.Fatalf("seed %d: (a) %s in a merged block reads\n%s\nbut alone\n%s", seed, name, a, b)
				}
				// (c) the header counts are the repository's own.
				if len(mb.Names) > 1 {
					rc, fc := changesOf(name, g)
					if counts := changeCounts(rc, fc); counts != mb.Right {
						t.Fatalf("seed %d: (c) %s has %q, its merged block says %q each", seed, name, counts, mb.Right)
					}
				}
			}

			// (d) the note names exactly the parts the body hides that differ.
			var wantDiffer []ui.UnseenPart
			first := g.unseen[mb.Names[0]]
			for _, part := range []struct {
				part   ui.UnseenPart
				differ func(genUnseen) bool
			}{
				{ui.UnseenSettings, func(u genUnseen) bool { return u.settings != first.settings }},
				{ui.UnseenFiles, func(u genUnseen) bool { return u.files != first.files }},
				{ui.UnseenDelivery, func(u genUnseen) bool { return u.delivery != first.delivery }},
			} {
				for _, name := range mb.Names[1:] {
					if part.differ(g.unseen[name]) {
						wantDiffer = append(wantDiffer, part.part)
						break
					}
				}
			}
			if !reflect.DeepEqual(mb.Differ, wantDiffer) {
				t.Fatalf("seed %d: (d) block of %v notes %v, want %v", seed, mb.Names, mb.Differ, wantDiffer)
			}
			for _, part := range wantDiffer {
				noted[part]++
			}
		}
	}
	for _, part := range []ui.UnseenPart{ui.UnseenSettings, ui.UnseenFiles, ui.UnseenDelivery} {
		if noted[part] < 10 {
			t.Errorf("generated plans noted part %d only %d times; the generator should merge repositories that differ in it", part, noted[part])
		}
	}
}
