package infra

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/babarot/gh-infra/internal/fileset"
	"github.com/babarot/gh-infra/internal/manifest"
	"github.com/babarot/gh-infra/internal/repository"
	"github.com/babarot/gh-infra/internal/ui"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata/plan")

// planScene is the input of one plan body in testdata/plan.
type planScene struct {
	repo     []repository.Change
	files    []fileset.Change
	notes    map[string][]string
	specs    []*manifest.Repository // what apply reads beyond the changes
	showDiff bool
}

// TestPlanBodyGolden compares the whole plan body of each scene with its
// golden file, without styles or a terminal width, so a change to how the
// body looks shows up as a diff of testdata/plan. Rewrite the files with
//
//	go test ./internal/infra -run TestPlanBodyGolden -update
func TestPlanBodyGolden(t *testing.T) {
	for name, scene := range planScenes() {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			p := ui.NewStandardPrinterWith(&buf, &buf)
			printPlanWithNotes(p, scene.repo, scene.files, scene.notes, unseenOf(scene.specs), scene.showDiff)
			p.Summary("Plan: end of body") // flushes the blank line a block leaves pending

			path := filepath.Join("testdata", "plan", name+".golden")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if got := buf.String(); got != string(want) {
				t.Errorf("plan body differs from %s (run with -update to accept)\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
			}
		})
	}
}

func planScenes() map[string]planScene {
	scenes := map[string]planScene{
		"many":       manyScene(),
		"labels":     labelsScene(),
		"fileset":    fileSetScene(),
		"template":   templateScene(),
		"ruleset":    rulesetScene(),
		"fileset-id": fileSetIDScene(),
		"flat":       flatScene(),
		"mixed":      mixedScene(),
		"wide":       wideScene(),
		"lifecycle":  lifecycleScene(),
	}
	diff := fileSetScene()
	diff.showDiff = true
	scenes["fileset-diff"] = diff
	tmplDiff := templateScene()
	tmplDiff.showDiff = true
	scenes["template-diff"] = tmplDiff
	return scenes
}

func update(name, field string, old, new any) repository.Change {
	return repository.Change{Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: name, Field: field, OldValue: old, NewValue: new}
}

// group is a change like the ones diffContext.group makes (features,
// merge_strategy): a field whose children are shown under it.
func group(name, field string, details ...repository.Change) repository.Change {
	return repository.Change{Type: repository.ChangeUpdate, Resource: manifest.ResourceRepository, Name: name, Field: field, Details: details}
}

func child(field string, old, new any) repository.Change {
	return repository.Change{Type: repository.ChangeUpdate, Field: field, OldValue: old, NewValue: new}
}

func createRepo(name string) repository.Change {
	return repository.Change{Type: repository.ChangeCreate, Resource: manifest.ResourceRepository, Name: name, Field: "repository", NewValue: name}
}

func fileChange(target, path string, typ fileset.ChangeType, current, desired, via string) fileset.Change {
	return fileset.Change{FileSetID: "template", Target: target, Path: path, Type: typ, Current: current, Desired: desired, Via: via}
}

func manyScene() planScene {
	var s planScene
	for i := 1; i <= 40; i++ {
		name := fmt.Sprintf("acme/repo-%02d", i)
		switch i {
		case 7, 19, 33:
			s.repo = append(s.repo,
				group(name, "features", child("wiki", true, false)),
				group(name, "merge_strategy", child("allow_squash_merge", false, true)))
		case 38:
			s.repo = append(s.repo, createRepo(name))
		default:
			s.repo = append(s.repo, repository.Change{Type: repository.ChangeNoOp, Name: name})
		}
	}
	return s
}

func labelChanges(name, oldColor string) []repository.Change {
	return []repository.Change{
		{Type: repository.ChangeCreate, Resource: manifest.ResourceLabel, Name: name, Field: "dependencies", NewValue: `#0366d6 "Pull requests that update a dependency"`},
		{Type: repository.ChangeUpdate, Resource: manifest.ResourceLabel, Name: name, Field: "bug",
			Details: []repository.Change{child("color", oldColor, "d73a4a")}},
	}
}

func labelsScene() planScene {
	var s planScene
	for i, r := range []string{"gh-infra", "gomi", "afx", "enhancd", "github-labeler", "pinentry-touchid", "dotfiles", "mo",
		"gh-news", "herdr", "tellme", "ghq-fzf", "zsh-history", "go-ci", "terraform-modules", "wiki"} {
		name := "babarot/" + r
		switch {
		case r == "gomi":
			s.repo = append(s.repo, labelChanges(name, "ff0000")...)
		case r == "afx":
			s.repo = append(s.repo, update(name, "description", "afx", "Package manager for CLI"))
			s.repo = append(s.repo, labelChanges(name, "ee0701")...)
		case i >= 14:
			s.repo = append(s.repo, repository.Change{Type: repository.ChangeNoOp, Name: name})
		default:
			s.repo = append(s.repo, labelChanges(name, "ee0701")...)
		}
	}
	return s
}

var services = []string{"auth", "billing", "catalog", "checkout", "gateway", "inventory", "mailer", "orders", "payments", "search", "shipping", "users"}

const (
	ciCurrent   = "name: ci\non: [push]\njobs:\n  test:\n    steps:\n      - uses: actions/setup-go@v5\n        with:\n          go-version: \"1.22\"\n      - run: make lint\n"
	ciDesired   = "name: ci\non: [push]\njobs:\n  test:\n    steps:\n      - uses: actions/setup-go@v5\n        with:\n          go-version: \"1.23\"\n          cache: true\n      - run: make lint\n"
	ciDesired2  = "name: ci\non: [push]\njobs:\n  test:\n    steps:\n      - uses: actions/setup-go@v5\n        with:\n          go-version: \"1.23\"\n          cache: true\n      - run: make lint test\n"
	dependabot  = "version: 2\nupdates:\n  - package-ecosystem: gomod\n    schedule: { interval: weekly }\n"
	readmeTitle = "# svc-%s\n"
	readmeBadge = "# svc-%s [![CI](https://github.com/acme/svc-%s/actions/workflows/ci.yaml/badge.svg)]\n"
)

func fileSetScene() planScene {
	var s planScene
	for i, svc := range services {
		name := "acme/svc-" + svc
		desired := ciDesired
		if i == 3 || i == 8 {
			desired = ciDesired2
		}
		s.files = append(s.files,
			fileChange(name, ".github/workflows/ci.yaml", fileset.ChangeUpdate, ciCurrent, desired, manifest.ViaPullRequest),
			fileChange(name, ".github/dependabot.yaml", fileset.ChangeCreate, "", dependabot, manifest.ViaPullRequest))
	}
	return s
}

// templateScene has a file whose contents differ per repository while its
// line counts do not.
func templateScene() planScene {
	var s planScene
	for _, svc := range services {
		name := "acme/svc-" + svc
		s.files = append(s.files,
			fileChange(name, ".github/workflows/ci.yaml", fileset.ChangeUpdate, ciCurrent, ciDesired, manifest.ViaPullRequest),
			fileChange(name, "README.md", fileset.ChangeUpdate, fmt.Sprintf(readmeTitle, svc), fmt.Sprintf(readmeBadge, svc, svc), manifest.ViaPullRequest))
	}
	return s
}

// rulesetScene creates the same-looking ruleset in every repository; the
// plan shows its bypass actors only as a count, and two repositories let
// another team bypass it.
func rulesetScene() planScene {
	var s planScene
	for _, svc := range services {
		name := "acme/svc-" + svc
		team := "platform"
		if svc == "billing" || svc == "payments" {
			team = "security"
		}
		s.specs = append(s.specs, &manifest.Repository{
			Metadata: manifest.RepositoryMetadata{Owner: "acme", Name: "svc-" + svc},
			Spec: manifest.RepositorySpec{Rulesets: []manifest.Ruleset{{
				Name:         "main",
				BypassActors: []manifest.RulesetBypassActor{{Team: team, BypassMode: "always"}},
			}}},
		})
		s.repo = append(s.repo, repository.Change{
			Type: repository.ChangeCreate, Resource: "Ruleset[main]", Name: name, Field: "ruleset", NewValue: "main",
			Details: []repository.Change{
				{Type: repository.ChangeCreate, Field: "enforcement", NewValue: "active"},
				{Type: repository.ChangeCreate, Field: "bypass_actors", NewValue: "1 actors"},
				{Type: repository.ChangeCreate, Field: "rules.pull_request", NewValue: "enabled"},
				{Type: repository.ChangeCreate, Field: "conditions", NewValue: "~DEFAULT_BRANCH"},
			},
		})
	}
	return s
}

// fileSetIDScene has two FileSets with the same ID delivering the same
// files from different pull request branches.
func fileSetIDScene() planScene {
	newFileSet := func(branch string) *manifest.FileSet {
		return &manifest.FileSet{
			Metadata: manifest.FileSetMetadata{Owner: "acme", Name: "template-sync"},
			Spec:     manifest.FileSetSpec{Via: manifest.ViaPullRequest, Branch: branch},
		}
	}
	first, second := newFileSet("gh-infra/template-sync"), newFileSet("chore/template")
	s := fileSetScene()
	for i := range s.files {
		s.files[i].Desired = ciDesired
		if s.files[i].Path == ".github/dependabot.yaml" {
			s.files[i].Desired = dependabot
		}
		s.files[i].FileSet = first
		if i >= len(s.files)/2 {
			s.files[i].FileSet = second
		}
	}
	return s
}

func flatScene() planScene {
	const name = "babarot/gh-infra"
	return planScene{repo: []repository.Change{
		update(name, "description", "gh infra", "Declarative GitHub infrastructure"),
		group(name, "features", child("wiki", true, false)),
		group(name, "merge_strategy", child("allow_squash_merge", false, true), child("auto_delete_head_branches", false, true)),
		group(name, "security", child("secret_scanning", "disabled", "enabled")),
		{Type: repository.ChangeUpdate, Resource: "Ruleset[main]", Name: name, Field: "ruleset", NewValue: "main",
			Details: []repository.Change{child("enforcement", "evaluate", "active")}},
		{Type: repository.ChangeUpdate, Resource: manifest.ResourceLabel, Name: name, Field: "bug",
			Details: []repository.Change{child("color", "ee0701", "d73a4a")}},
		{Type: repository.ChangeUpdate, Resource: "BranchProtection[main]", Name: name, Field: "branch_protection", NewValue: "main",
			Details: []repository.Change{child("required_reviews", 1, 2)}},
	}}
}

func mixedScene() planScene {
	s := planScene{repo: []repository.Change{
		update("acme/api", "description", "api server", "API server for acme"),
		update("acme/api", "topics", []string{}, []string{"go", "api", "grpc"}),
		group("acme/api", "features", child("wiki", true, false)),
		group("acme/api", "merge_strategy", child("allow_squash_merge", false, true), child("auto_delete_head_branches", false, true)),
		update("acme/web", "description", "web", "Web frontend"),
		update("acme/web", "topics", []string{}, []string{"typescript", "react"}),
		group("acme/web", "features", child("wiki", true, false)),
		group("acme/web", "merge_strategy", child("allow_squash_merge", false, true), child("auto_delete_head_branches", false, true)),
		{Type: repository.ChangeNoOp, Name: "acme/cli"},
		createRepo("acme/new-app"),
	}}
	s.files = []fileset.Change{
		fileChange("acme/api", "LICENSE", fileset.ChangeUpdate, "Copyright (c) 2024 acme\n", "Copyright (c) 2026 acme\n", manifest.ViaPush),
		fileChange("acme/api", ".github/workflows/ci.yaml", fileset.ChangeCreate, "", ciDesired, manifest.ViaPush),
		fileChange("acme/web", ".github/CODEOWNERS", fileset.ChangeCreate, "", "* @acme/web\n", manifest.ViaPullRequest),
	}
	return s
}

func wideScene() planScene {
	var s planScene
	for i := 1; i <= 30; i++ {
		s.repo = append(s.repo, group(fmt.Sprintf("org/team-%02d-service", i), "features", child("wiki", true, false)))
	}
	return s
}

// lifecycleScene covers the blocks the other scenes do not: a created
// repository with notes and files pushed to it, a destroyed one, and a
// file whose mode changes.
func lifecycleScene() planScene {
	s := planScene{
		repo: []repository.Change{
			createRepo("acme/new-app"),
			{Type: repository.ChangeCreate, Resource: manifest.ResourceRepository, Name: "acme/new-app", Field: "description", NewValue: "New app"},
			{Type: repository.ChangeDelete, Resource: manifest.ResourceRepository, Name: "acme/old-app", Field: "repository", OldValue: "acme/old-app"},
			update("acme/tools", "homepage", "", "https://acme.example.com/tools"),
		},
		notes: map[string][]string{"acme/new-app": {"branch protection for main is applied by a later plan, once the branch exists"}},
	}
	push := fileChange("acme/new-app", "README.md", fileset.ChangeCreate, "", "# new-app\n", manifest.ViaPush)
	push.NoCommits = true
	script := fileChange("acme/tools", "bin/setup", fileset.ChangeUpdate, "#!/bin/sh\n", "#!/bin/sh\nset -eu\n", manifest.ViaPush)
	script.Mode, script.CurrentMode = "100755", "100644"
	s.files = []fileset.Change{push, script}
	return s
}
