package repository

import (
	"encoding/json"
	"slices"

	"github.com/babarot/gh-infra/internal/gh"
	"github.com/babarot/gh-infra/internal/manifest"
)

// fakePages is a GitHub Pages site in the shape of the Pages API.
type fakePages struct {
	BuildType     string          `json:"build_type"`
	Source        fakePagesSource `json:"source"`
	CNAME         string          `json:"cname"`
	HTTPSEnforced bool            `json:"https_enforced"`
	// CertificateState is the state of the certificate of the site's custom
	// domain, "" when GitHub reports none (as for a site without a domain).
	// GitHub issues one in its own time after the site gets a domain; the
	// fake never does, a test sets it.
	CertificateState string `json:"certificate_state"`
}

type fakePagesSource struct {
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

func (f *fakeGitHub) apiPages(method string, body []byte) ([]byte, bool, error) {
	p := f.s.Pages
	switch method {
	case "GET":
		if p == nil {
			return nil, false, fakeStatus(gh.ErrNotFound, "Not Found")
		}
		resp := map[string]any{
			"url":            "https://api.github.com/repos/" + f.fullName() + "/pages",
			"status":         "built",
			"build_type":     p.BuildType,
			"source":         p.Source,
			"cname":          nil,
			"https_enforced": p.HTTPSEnforced,
			"public":         true,
		}
		if p.CNAME != "" {
			resp["cname"] = p.CNAME
		}
		if p.CertificateState != "" {
			resp["https_certificate"] = map[string]any{"state": p.CertificateState}
		}
		return mustJSON(resp), false, nil
	case "POST":
		if p != nil {
			return nil, false, fakeStatus(gh.ErrConflict, "GitHub Pages is already enabled.")
		}
		// GitHub Free has Pages only on public repositories.
		if f.s.Repo.Visibility != manifest.VisibilityPublic && (f.s.Org.Plan == "" || f.s.Org.Plan == "free") {
			return nil, false, fakeStatus(gh.ErrValidation, "Your current plan does not support GitHub Pages for this repository.")
		}
		var req struct {
			BuildType string           `json:"build_type"`
			Source    *fakePagesSource `json:"source"`
		}
		if err := strictJSON(body, &req); err != nil {
			return nil, false, err
		}
		np := &fakePages{BuildType: req.BuildType}
		switch req.BuildType {
		case manifest.PagesBuildTypeWorkflow:
			if req.Source != nil {
				return nil, false, errUnknown // gh-infra does not send one
			}
			np.Source = fakePagesSource{Branch: f.s.Repo.DefaultBranch, Path: "/"}
		case manifest.PagesBuildTypeLegacy:
			if err := f.checkPagesSource(req.Source); err != nil {
				return nil, false, err
			}
			np.Source = *req.Source
		default:
			return nil, false, fakeStatus(gh.ErrValidation, "build_type is invalid")
		}
		f.s.Pages = np
		return mustJSON(np), true, nil
	case "PUT":
		if p == nil {
			return nil, false, fakeStatus(gh.ErrNotFound, "Not Found")
		}
		var req map[string]json.RawMessage
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, false, err
		}
		next := *p
		for k, v := range req {
			switch k {
			case "build_type":
				if err := json.Unmarshal(v, &next.BuildType); err != nil {
					return nil, false, err
				}
			case "source":
				var src fakePagesSource
				if err := strictJSON(v, &src); err != nil {
					return nil, false, err
				}
				if err := f.checkPagesSource(&src); err != nil {
					return nil, false, err
				}
				next.Source = src
			case "cname":
				var cname *string
				if err := json.Unmarshal(v, &cname); err != nil {
					return nil, false, err
				}
				next.CNAME = ""
				if cname != nil {
					next.CNAME = *cname
				}
				if next.CNAME != p.CNAME {
					// A new domain needs a new certificate.
					next.CertificateState = ""
					if next.CNAME != "" {
						next.CertificateState = "new"
					}
				}
			case "https_enforced":
				if err := json.Unmarshal(v, &next.HTTPSEnforced); err != nil {
					return nil, false, err
				}
			default:
				return nil, false, errUnknown
			}
		}
		// gh-infra always sends the source with a move to legacy: the
		// last one GitHub kept may not be the one wanted.
		if next.BuildType == manifest.PagesBuildTypeLegacy && p.BuildType != manifest.PagesBuildTypeLegacy && req["source"] == nil {
			return nil, false, errUnknown
		}
		// A site without a custom domain is served from GitHub's, which
		// needs no certificate of its own: GitHub may report none. Whether
		// GitHub checks the domain before or after the PUT is not known, so
		// the fake accepts only what both would.
		hasCertificate := func(s *fakePages) bool { return s.CNAME == "" || s.CertificateState == certificateApproved }
		if next.HTTPSEnforced && !p.HTTPSEnforced && (!hasCertificate(p) || !hasCertificate(&next)) {
			return nil, false, fakeStatus(gh.ErrNotFound, "The certificate does not exist yet")
		}
		f.s.Pages = &next
		return nil, true, nil
	}
	return nil, false, errUnknown
}

func (f *fakeGitHub) checkPagesSource(src *fakePagesSource) error {
	if src == nil || src.Branch == "" || (src.Path != "/" && src.Path != "/docs") {
		return fakeStatus(gh.ErrValidation, "source is invalid")
	}
	if !slices.Contains(f.s.Branches, src.Branch) {
		return fakeStatus(gh.ErrValidation, "The branch "+src.Branch+" does not exist")
	}
	return nil
}

// strictJSON decodes data into v, failing on a field v does not have.
func strictJSON(data []byte, v any) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out, _ := json.Marshal(v)
	var known map[string]json.RawMessage
	_ = json.Unmarshal(out, &known)
	for k := range raw {
		if _, ok := known[k]; !ok {
			return errUnknown
		}
	}
	return json.Unmarshal(data, v)
}
