package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/babarot/gh-infra/internal/manifest"
)

// Fields of a Pages change. Each is a detail of the one change that enables
// the site or updates it; apply sends what the details say.
const (
	pagesFieldBuildType     = "build_type"
	pagesFieldSource        = "source"
	pagesFieldCNAME         = "cname"
	pagesFieldHTTPSEnforced = "https_enforced"
)

// certificateApproved is the state of a Pages certificate GitHub has issued.
const certificateApproved = "approved"

// pagesHTTPSDeferred is what a plan shows for https_enforced: true that this
// run cannot send: GitHub refuses it for a custom domain until it has issued
// the domain's certificate. apply sends nothing for it, and a later plan shows it again.
const pagesHTTPSDeferred = "true (once GitHub has issued the certificate)"

func formatPagesSource(branch, path string) string {
	return branch + ":" + path
}

// diffPages plans the Pages site the manifest declares: a create that
// enables it, or an update of the fields that differ. A site that does not
// exist yet is enabled with the declared build type and source, and then
// given its domain.
func diffPages(name string, desired *manifest.Repository, current *CurrentState) []Change {
	p := desired.Spec.Pages
	if p == nil {
		return nil
	}
	cur := current.Pages
	var details []Change
	add := func(field string, old, new any) {
		typ := ChangeUpdate
		if !cur.Enabled {
			typ = ChangeCreate
		}
		details = append(details, Change{Type: typ, Field: field, OldValue: old, NewValue: new})
	}

	if p.BuildType != nil && (!cur.Enabled || *p.BuildType != cur.BuildType) {
		add(pagesFieldBuildType, cur.BuildType, *p.BuildType)
	}
	// A source is declared only with legacy (validate). Moving a site to
	// legacy sends its source too, as GitHub's last one may be stale.
	if p.Legacy() {
		want := formatPagesSource(p.Source.Branch, p.Source.SourcePath())
		have := formatPagesSource(cur.Source.Branch, cur.Source.Path)
		if !cur.Enabled || want != have || cur.BuildType != manifest.PagesBuildTypeLegacy {
			add(pagesFieldSource, have, want)
		}
	}
	cnameChanges := p.CNAME != nil && (*p.CNAME != cur.CNAME)
	if cnameChanges && (cur.Enabled || *p.CNAME != "") {
		add(pagesFieldCNAME, cur.CNAME, *p.CNAME)
	}
	// The domain the site has after apply.
	cname := cur.CNAME
	if p.CNAME != nil {
		cname = *p.CNAME
	}
	if h := p.HTTPSEnforced; h != nil && (!cur.Enabled || *h != cur.HTTPSEnforced) {
		switch {
		case *h && cname != "" && (!cur.Enabled || cnameChanges || cur.CertificateState != certificateApproved):
			// GitHub issues the certificate of a custom domain after the
			// site has it, in its own time. A site without one is served
			// from GitHub's domain, which needs no certificate of its own
			// (GitHub may report none).
			add(pagesFieldHTTPSEnforced, cur.HTTPSEnforced, pagesHTTPSDeferred)
		default:
			add(pagesFieldHTTPSEnforced, cur.HTTPSEnforced, *h)
		}
	}
	if len(details) == 0 {
		return nil
	}
	c := Change{
		Type:     ChangeUpdate,
		Resource: manifest.ResourcePages,
		Name:     name,
		Field:    "pages",
		Details:  details,
	}
	if !cur.Enabled {
		c.Type = ChangeCreate
	}
	return []Change{c}
}

// validatePagesDependencies fails what GitHub would refuse: enabling a site
// without a build type, and building one from a branch that does not exist.
func validatePagesDependencies(desired *manifest.Repository, current *CurrentState) error {
	p := desired.Spec.Pages
	if p == nil || current.IsNew {
		return nil
	}
	cur := current.Pages
	if !cur.Enabled && p.BuildType == nil {
		return fmt.Errorf("pages: GitHub Pages is not enabled on %s; set pages.build_type to enable it", desired.Metadata.FullName())
	}
	if p.Legacy() && !cur.SourceBranchExists {
		changes := diffPages(desired.Metadata.FullName(), desired, current)
		if len(changes) > 0 && hasDetail(changes[0], func(d Change) bool { return d.Field == pagesFieldSource }) {
			return fmt.Errorf("pages.source.branch: branch %q does not exist in %s", p.Source.Branch, desired.Metadata.FullName())
		}
	}
	return nil
}

// checkPagesPlan fails when the plan enables a site on a private repository
// and the owner's GitHub plan does not allow it (GitHub Free allows Pages
// only on public repositories). A site that exists shows the plan allows it.
func (p *Processor) checkPagesPlan(ctx context.Context, desired *manifest.Repository, current *CurrentState, changes []Change) error {
	if desired.Spec.Pages == nil || current.Pages.Enabled {
		return nil
	}
	enables := false
	for _, c := range changes {
		if c.Resource == manifest.ResourcePages && c.Type == ChangeCreate {
			enables = true
		}
	}
	if !enables || effectiveVisibility(desired, current) == manifest.VisibilityPublic {
		return nil
	}
	owner := desired.Metadata.Owner
	plan, err := p.ownerPlan(ctx, owner)
	if err != nil {
		return err
	}
	switch plan {
	case "free":
		return fmt.Errorf("pages: GitHub's free plan allows Pages only on public repositories")
	case "pro", "team", "enterprise":
		return nil
	default:
		return fmt.Errorf("pages: cannot tell whether the plan of %s allows Pages on a non-public repository (GitHub Free does not); plan with a token that can read the plan (an organization owner's, or the user's own with the user scope), or enable Pages once on GitHub", owner)
	}
}

// pagesCreateNotes says that a repository this run creates gets its Pages
// site from a later plan: it has no commits yet to build one from.
func pagesCreateNotes(desired *manifest.Repository) []string {
	if desired.Spec.Pages == nil {
		return nil
	}
	return []string{"pages is enabled by a later plan, once the repository has commits"}
}

// pagesChangeNotes explains the https_enforced a plan shows but this run
// does not send.
func pagesChangeNotes(changes []Change) []string {
	var notes []string
	for _, c := range changes {
		if c.Resource != manifest.ResourcePages {
			continue
		}
		if hasDetail(c, func(d Change) bool { return d.Field == pagesFieldHTTPSEnforced && d.NewValue == pagesHTTPSDeferred }) {
			notes = append(notes, "pages: https_enforced is applied by a later plan, once GitHub has issued the certificate for the site's domain")
		}
	}
	return notes
}

// applyPages enables the site (POST) and then sends the rest of the details
// (PUT), or, for a site that exists, sends the details that changed.
func (p *Processor) applyPages(ctx context.Context, c Change, repo *manifest.Repository) error {
	pg := repo.Spec.Pages
	if pg == nil {
		return nil
	}
	owner, name := repo.Metadata.Owner, repo.Metadata.Name
	fullName := owner + "/" + name
	path := fmt.Sprintf("repos/%s/%s/pages", owner, name)

	source := func() map[string]any {
		return map[string]any{"branch": pg.Source.Branch, "path": pg.Source.SourcePath()}
	}
	put := map[string]any{}
	for _, d := range c.Details {
		switch d.Field {
		case pagesFieldBuildType:
			// A move to legacy has a source detail too (diffPages).
			if c.Type != ChangeCreate {
				put["build_type"] = *pg.BuildType
			}
		case pagesFieldSource:
			if c.Type != ChangeCreate {
				put["source"] = source()
			}
		case pagesFieldCNAME:
			if *pg.CNAME == "" {
				put["cname"] = nil
			} else {
				put["cname"] = *pg.CNAME
			}
		case pagesFieldHTTPSEnforced:
			if v, ok := d.NewValue.(bool); ok {
				put["https_enforced"] = v
			}
		}
	}

	send := func(method string, payload map[string]any) error {
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = p.runner.RunWithStdin(ctx, body,
			"api", path,
			"--method", method,
			"--header", "Content-Type: application/json",
			"--input", "-",
		)
		return wrapError(err, fullName, "pages")
	}

	if c.Type == ChangeCreate {
		post := map[string]any{"build_type": *pg.BuildType}
		if pg.Legacy() {
			post["source"] = source()
		}
		if err := send("POST", post); err != nil {
			return err
		}
	}
	if len(put) == 0 {
		return nil
	}
	return send("PUT", put)
}
