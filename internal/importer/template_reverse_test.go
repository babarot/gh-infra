package importer

import "testing"

func TestReverseTemplateContent_SimplePlaceholder(t *testing.T) {
	template := "module github.com/<% .Repo.FullName %>\n\ngo 1.26.0\n"
	remote := "module github.com/hoge/fuga\n\ngo 1.27.0\n"

	trace, ok := prepareTemplateReverse(template, "hoge/fuga", nil)
	if !ok {
		t.Fatal("expected prepareTemplateReverse to succeed")
	}
	got, ok := reverseRenderedTemplate(trace, remote)
	if !ok {
		t.Fatal("expected reverseRenderedTemplate to succeed")
	}

	want := "module github.com/<% .Repo.FullName %>\n\ngo 1.27.0\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReverseTemplateContent_UnsupportedControlSyntax(t *testing.T) {
	template := "<% if .Repo.Name %>enabled<% end %>\n"

	trace, ok := prepareTemplateReverse(template, "org/repo", nil)
	if ok {
		t.Fatalf("expected prepareTemplateReverse to reject unsupported syntax, got %+v", trace)
	}
}

func TestReverseTemplateContent_ConsecutivePlaceholders(t *testing.T) {
	template := "<% .Repo.Owner %><% .Repo.Name %>\n"
	remote := "babarotgh-infra\n"

	trace, ok := prepareTemplateReverse(template, "babarot/gh-infra", nil)
	if !ok {
		t.Fatal("expected prepareTemplateReverse to succeed")
	}
	got, ok := reverseRenderedTemplate(trace, remote)
	if !ok {
		t.Fatal("expected reverseRenderedTemplate to support consecutive placeholders")
	}
	if got != template {
		t.Fatalf("got %q, want %q", got, template)
	}
}

func TestReverseTemplateContent_ChangedVarsPlaceholderRejected(t *testing.T) {
	template := "GO_VERSION=<% .Vars.go_version %>\n"
	remote := "GO_VERSION=1.27.3\n"

	trace, ok := prepareTemplateReverse(template, "org/repo", map[string]string{"go_version": "1.26.1"})
	if !ok {
		t.Fatal("expected prepareTemplateReverse to succeed")
	}
	if _, ok := reverseRenderedTemplate(trace, remote); ok {
		t.Fatal("expected reverseRenderedTemplate to reject changed .Vars placeholders")
	}
}

func TestReverseRenderedTemplate_RejectsRigidLiteralRewrite(t *testing.T) {
	template := "PREFIX=<% .Repo.Name %>\n"
	remote := "TOTALLY_DIFFERENT=repo\n"

	trace, ok := prepareTemplateReverse(template, "owner/repo", nil)
	if !ok {
		t.Fatal("expected prepareTemplateReverse to succeed")
	}
	if _, ok := reverseRenderedTemplate(trace, remote); ok {
		t.Fatal("expected reverseRenderedTemplate to reject rigid literal rewrite")
	}
}
