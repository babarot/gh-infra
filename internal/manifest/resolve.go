package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/babarot/gh-infra/internal/gh"
)

// parseIDPrefix checks if s has the form "id:<number>" and returns the numeric ID.
// This is used to handle unresolvable entities (private apps, deleted teams, etc.)
// that were exported with a numeric fallback by import.
func parseIDPrefix(s string) (int, bool) {
	if !strings.HasPrefix(s, "id:") {
		return 0, false
	}
	id, err := strconv.Atoi(s[3:])
	if err != nil {
		return 0, false
	}
	return id, true
}

// Well-known built-in repository role IDs.
var roleIDs = map[string]int{
	"admin":    5,
	"write":    4,
	"maintain": 2,
}

// roleNames is the reverse mapping of roleIDs.
var roleNames = map[int]string{
	5: "admin",
	4: "write",
	2: "maintain",
}

// ResolvedBypassActor is a bypass actor with numeric IDs ready for the GitHub API.
type ResolvedBypassActor struct {
	ActorID    int
	ActorType  string
	BypassMode string
}

// ResolvedStatusCheck is a status check with optional numeric App ID.
type ResolvedStatusCheck struct {
	Context       string
	IntegrationID int // 0 = any provider
}

// Resolver resolves human-readable names to GitHub API numeric IDs.
type Resolver struct {
	runner   gh.Runner
	owner    string
	appCache map[string]int // slug → App ID
}

// NewResolver creates a new Resolver.
func NewResolver(runner gh.Runner, owner string) *Resolver {
	return &Resolver{
		runner:   runner,
		owner:    owner,
		appCache: make(map[string]int),
	}
}

// ResolveBypassActors converts name-based bypass actors to numeric ID form.
func (r *Resolver) ResolveBypassActors(ctx context.Context, actors []RulesetBypassActor) ([]ResolvedBypassActor, error) {
	var resolved []ResolvedBypassActor
	for _, a := range actors {
		ra, err := r.resolveBypassActor(ctx, a)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, ra)
	}
	return resolved, nil
}

func (r *Resolver) resolveBypassActor(ctx context.Context, a RulesetBypassActor) (ResolvedBypassActor, error) {
	switch {
	case a.Role != "":
		id, ok := roleIDs[strings.ToLower(a.Role)]
		if !ok {
			return ResolvedBypassActor{}, fmt.Errorf("unknown role %q (must be admin, write, or maintain)", a.Role)
		}
		return ResolvedBypassActor{ActorID: id, ActorType: "RepositoryRole", BypassMode: a.BypassMode}, nil

	case a.Team != "":
		id, err := r.resolveTeamID(ctx, a.Team)
		if err != nil {
			return ResolvedBypassActor{}, fmt.Errorf("resolve team %q: %w", a.Team, err)
		}
		return ResolvedBypassActor{ActorID: id, ActorType: "Team", BypassMode: a.BypassMode}, nil

	case a.App != "":
		id, err := r.ResolveAppID(ctx, a.App)
		if err != nil {
			return ResolvedBypassActor{}, fmt.Errorf("resolve app %q: %w", a.App, err)
		}
		return ResolvedBypassActor{ActorID: id, ActorType: "Integration", BypassMode: a.BypassMode}, nil

	case a.OrgAdmin != nil && *a.OrgAdmin:
		return ResolvedBypassActor{ActorID: 1, ActorType: "OrganizationAdmin", BypassMode: a.BypassMode}, nil

	case a.CustomRole != "":
		id, err := r.resolveCustomRoleID(ctx, a.CustomRole)
		if err != nil {
			return ResolvedBypassActor{}, fmt.Errorf("resolve custom role %q: %w", a.CustomRole, err)
		}
		return ResolvedBypassActor{ActorID: id, ActorType: "RepositoryRole", BypassMode: a.BypassMode}, nil

	default:
		return ResolvedBypassActor{}, fmt.Errorf("bypass actor must specify one of: role, team, app, org-admin, or custom-role")
	}
}

// ResolveStatusChecks converts name-based status checks to numeric ID form.
func (r *Resolver) ResolveStatusChecks(ctx context.Context, checks []RulesetStatusCheck) ([]ResolvedStatusCheck, error) {
	var resolved []ResolvedStatusCheck
	for _, c := range checks {
		rc := ResolvedStatusCheck{Context: c.Context}
		if c.App != "" {
			id, err := r.ResolveAppID(ctx, c.App)
			if err != nil {
				return nil, fmt.Errorf("resolve app %q for context %q: %w", c.App, c.Context, err)
			}
			rc.IntegrationID = id
		}
		resolved = append(resolved, rc)
	}
	return resolved, nil
}

// ResolveAppID resolves a GitHub App slug to its App ID.
// Accepts "id:<number>" form for private/unresolvable apps.
func (r *Resolver) ResolveAppID(ctx context.Context, slug string) (int, error) {
	if id, ok := parseIDPrefix(slug); ok {
		return id, nil
	}
	if id, ok := r.appCache[slug]; ok {
		return id, nil
	}
	out, err := r.runner.Run(ctx, "api", fmt.Sprintf("apps/%s", slug))
	if err != nil {
		return 0, fmt.Errorf("fetch app %q: %w", slug, err)
	}
	var app struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(out, &app); err != nil {
		return 0, fmt.Errorf("parse app %q response: %w", slug, err)
	}
	r.appCache[slug] = app.ID
	return app.ID, nil
}

func (r *Resolver) resolveTeamID(ctx context.Context, slug string) (int, error) {
	if id, ok := parseIDPrefix(slug); ok {
		return id, nil
	}
	out, err := r.runner.Run(ctx, "api", fmt.Sprintf("orgs/%s/teams/%s", r.owner, slug))
	if err != nil {
		return 0, fmt.Errorf("fetch team %q: %w", slug, err)
	}
	var team struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(out, &team); err != nil {
		return 0, fmt.Errorf("parse team %q response: %w", slug, err)
	}
	return team.ID, nil
}

func (r *Resolver) resolveCustomRoleID(ctx context.Context, name string) (int, error) {
	if id, ok := parseIDPrefix(name); ok {
		return id, nil
	}
	out, err := r.runner.Run(ctx, "api", fmt.Sprintf("orgs/%s/custom-repository-roles", r.owner))
	if err != nil {
		return 0, fmt.Errorf("fetch custom roles: %w", err)
	}
	var resp struct {
		CustomRoles []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"custom_roles"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return 0, fmt.Errorf("parse custom roles response: %w", err)
	}
	for _, cr := range resp.CustomRoles {
		if strings.EqualFold(cr.Name, name) {
			return cr.ID, nil
		}
	}
	return 0, fmt.Errorf("custom role %q not found in org %s", name, r.owner)
}

// RoleNameFromID returns the role name for a known built-in RepositoryRole ID.
// Returns empty string for unknown IDs or non-RepositoryRole types.
func RoleNameFromID(actorID int, actorType string) string {
	if actorType != "RepositoryRole" {
		return ""
	}
	return roleNames[actorID]
}

// --- Reverse resolution (ID → name) for export/import ---

// ReverseBypassActor converts a numeric bypass actor back to name-based form.
func (r *Resolver) ReverseBypassActor(ctx context.Context, actorID int, actorType, bypassMode, repo string) RulesetBypassActor {
	switch actorType {
	case "RepositoryRole":
		if name, ok := roleNames[actorID]; ok {
			return RulesetBypassActor{Role: name, BypassMode: bypassMode}
		}
		// Unknown ID — likely a custom role, but we don't have the name without API call.
		// Fall back to custom-role with the ID as placeholder.
		return RulesetBypassActor{CustomRole: fmt.Sprintf("id:%d", actorID), BypassMode: bypassMode}

	case "Team":
		// Team requires reverse lookup: ID → slug
		slug, err := r.reverseTeamID(ctx, actorID)
		if err != nil {
			return RulesetBypassActor{Team: fmt.Sprintf("id:%d", actorID), BypassMode: bypassMode}
		}
		return RulesetBypassActor{Team: slug, BypassMode: bypassMode}

	case "Integration":
		slug, err := r.reverseAppID(ctx, actorID, repo)
		if err != nil {
			return RulesetBypassActor{App: fmt.Sprintf("id:%d", actorID), BypassMode: bypassMode}
		}
		return RulesetBypassActor{App: slug, BypassMode: bypassMode}

	case "OrganizationAdmin":
		return RulesetBypassActor{OrgAdmin: Ptr(true), BypassMode: bypassMode}

	default:
		return RulesetBypassActor{Role: fmt.Sprintf("unknown:%s:%d", actorType, actorID), BypassMode: bypassMode}
	}
}

// ReverseStatusCheck converts a numeric status check back to name-based form.
func (r *Resolver) ReverseStatusCheck(ctx context.Context, context_ string, integrationID int, repo string) RulesetStatusCheck {
	if integrationID == 0 {
		return RulesetStatusCheck{Context: context_}
	}
	slug, err := r.reverseAppID(ctx, integrationID, repo)
	if err != nil {
		return RulesetStatusCheck{Context: context_, App: fmt.Sprintf("id:%d", integrationID)}
	}
	return RulesetStatusCheck{Context: context_, App: slug}
}

func (r *Resolver) reverseTeamID(ctx context.Context, id int) (string, error) {
	// List teams and find by ID
	out, err := r.runner.Run(ctx, "api", fmt.Sprintf("orgs/%s/teams", r.owner), "--paginate")
	if err != nil {
		return "", err
	}
	var teams []struct {
		ID   int    `json:"id"`
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(out, &teams); err != nil {
		return "", err
	}
	for _, t := range teams {
		if t.ID == id {
			return t.Slug, nil
		}
	}
	return "", fmt.Errorf("team ID %d not found", id)
}

func (r *Resolver) reverseAppID(ctx context.Context, id int, repo string) (string, error) {
	// Check cache (reverse)
	for slug, cachedID := range r.appCache {
		if cachedID == id {
			return slug, nil
		}
	}
	// Discover App slug from check-runs on the repo's default branch
	out, err := r.runner.Run(ctx, "api",
		fmt.Sprintf("repos/%s/%s/commits/HEAD/check-runs", r.owner, repo),
		"--jq", ".check_runs",
	)
	if err == nil {
		var checkRuns []struct {
			App struct {
				ID   int    `json:"id"`
				Slug string `json:"slug"`
			} `json:"app"`
		}
		if err := json.Unmarshal(out, &checkRuns); err == nil {
			for _, cr := range checkRuns {
				r.appCache[cr.App.Slug] = cr.App.ID
				if cr.App.ID == id {
					return cr.App.Slug, nil
				}
			}
		}
	}
	return "", fmt.Errorf("app ID %d not found in check-runs for %s/%s", id, r.owner, repo)
}
