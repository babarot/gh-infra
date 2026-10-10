package repository

import (
	"encoding/json"
	"fmt"

	"github.com/babarot/gh-infra/internal/manifest"
)

// buildBranchProtectionPayload builds the payload to protect a branch that is
// not protected yet, from the manifest alone; there is nothing to keep.
func buildBranchProtectionPayload(bp *manifest.BranchProtection) map[string]any {
	payload := map[string]any{
		"enforce_admins":     derefBool(bp.EnforceAdmins),
		"restrictions":       nil,
		"allow_force_pushes": derefBool(bp.AllowForcePushes),
		"allow_deletions":    derefBool(bp.AllowDeletions),
	}
	if bp.RestrictPushes != nil && *bp.RestrictPushes {
		payload["restrictions"] = emptyRestrictions()
	}

	if bp.RequiredReviews != nil || bp.DismissStaleReviews != nil || bp.RequireCodeOwnerReviews != nil {
		payload["required_pull_request_reviews"] = overlayReviews(map[string]any{}, bp)
	} else {
		payload["required_pull_request_reviews"] = nil
	}

	if bp.RequireStatusChecks != nil {
		payload["required_status_checks"] = map[string]any{
			"strict":   bp.RequireStatusChecks.Strict,
			"contexts": contextsOrEmpty(bp.RequireStatusChecks.Contexts),
		}
	} else {
		payload["required_status_checks"] = nil
	}

	return payload
}

func emptyRestrictions() map[string]any {
	return map[string]any{"users": []string{}, "teams": []string{}, "apps": []string{}}
}

func contextsOrEmpty(contexts []string) []string {
	if contexts == nil {
		return []string{}
	}
	return contexts
}

func overlayReviews(reviews map[string]any, bp *manifest.BranchProtection) map[string]any {
	if bp.RequiredReviews != nil {
		reviews["required_approving_review_count"] = *bp.RequiredReviews
	}
	if bp.DismissStaleReviews != nil {
		reviews["dismiss_stale_reviews"] = *bp.DismissStaleReviews
	}
	if bp.RequireCodeOwnerReviews != nil {
		reviews["require_code_owner_reviews"] = *bp.RequireCodeOwnerReviews
	}
	return reviews
}

// protectionJSON is the GET branch protection response. Its shape differs
// from the PUT input: settings are {enabled} objects, and users, teams and
// apps are objects rather than logins and slugs.
type protectionJSON struct {
	RequiredStatusChecks *struct {
		Strict bool `json:"strict"`
		Checks []struct {
			Context string `json:"context"`
			AppID   *int   `json:"app_id"`
		} `json:"checks"`
	} `json:"required_status_checks"`
	EnforceAdmins              *enabledJSON `json:"enforce_admins"`
	RequiredPullRequestReviews *struct {
		DismissStaleReviews          bool        `json:"dismiss_stale_reviews"`
		RequireCodeOwnerReviews      bool        `json:"require_code_owner_reviews"`
		RequiredApprovingReviewCount int         `json:"required_approving_review_count"`
		RequireLastPushApproval      bool        `json:"require_last_push_approval"`
		DismissalRestrictions        *actorsJSON `json:"dismissal_restrictions"`
		BypassPullRequestAllowances  *actorsJSON `json:"bypass_pull_request_allowances"`
	} `json:"required_pull_request_reviews"`
	Restrictions                   *actorsJSON  `json:"restrictions"`
	RequiredLinearHistory          *enabledJSON `json:"required_linear_history"`
	AllowForcePushes               *enabledJSON `json:"allow_force_pushes"`
	AllowDeletions                 *enabledJSON `json:"allow_deletions"`
	BlockCreations                 *enabledJSON `json:"block_creations"`
	RequiredConversationResolution *enabledJSON `json:"required_conversation_resolution"`
	LockBranch                     *enabledJSON `json:"lock_branch"`
	AllowForkSyncing               *enabledJSON `json:"allow_fork_syncing"`
}

type enabledJSON struct {
	Enabled bool `json:"enabled"`
}

type actorsJSON struct {
	Users []struct {
		Login string `json:"login"`
	} `json:"users"`
	Teams []struct {
		Slug string `json:"slug"`
	} `json:"teams"`
	Apps []struct {
		Slug string `json:"slug"`
	} `json:"apps"`
}

// putForm returns the actors as the PUT input expects them: all three lists
// of logins and slugs, possibly empty.
func (a *actorsJSON) putForm() map[string]any {
	users, teams, apps := []string{}, []string{}, []string{}
	for _, u := range a.Users {
		users = append(users, u.Login)
	}
	for _, t := range a.Teams {
		teams = append(teams, t.Slug)
	}
	for _, app := range a.Apps {
		apps = append(apps, app.Slug)
	}
	return map[string]any{"users": users, "teams": teams, "apps": apps}
}

// protectionPutForm converts a GET branch protection response to the PUT
// input that keeps it as it is.
func protectionPutForm(current []byte) (map[string]any, error) {
	var cur protectionJSON
	if err := json.Unmarshal(current, &cur); err != nil {
		return nil, fmt.Errorf("parse current branch protection: %w", err)
	}

	payload := map[string]any{
		"required_status_checks":        nil,
		"enforce_admins":                cur.EnforceAdmins != nil && cur.EnforceAdmins.Enabled,
		"required_pull_request_reviews": nil,
		"restrictions":                  nil,
	}

	if sc := cur.RequiredStatusChecks; sc != nil {
		// checks keeps the app of each check. app_id null means any app,
		// which the PUT spells -1; leaving it out would bind the check to
		// the app that reported it last. contexts must not be sent too.
		checks := []map[string]any{}
		for _, c := range sc.Checks {
			appID := -1
			if c.AppID != nil {
				appID = *c.AppID
			}
			checks = append(checks, map[string]any{"context": c.Context, "app_id": appID})
		}
		payload["required_status_checks"] = map[string]any{"strict": sc.Strict, "checks": checks}
	}

	if r := cur.RequiredPullRequestReviews; r != nil {
		reviews := map[string]any{
			"dismiss_stale_reviews":           r.DismissStaleReviews,
			"require_code_owner_reviews":      r.RequireCodeOwnerReviews,
			"required_approving_review_count": r.RequiredApprovingReviewCount,
			"require_last_push_approval":      r.RequireLastPushApproval,
		}
		// Present only when enabled; sending an empty object would disable
		// it, and personal repositories do not accept it.
		if r.DismissalRestrictions != nil {
			reviews["dismissal_restrictions"] = r.DismissalRestrictions.putForm()
		}
		if r.BypassPullRequestAllowances != nil {
			reviews["bypass_pull_request_allowances"] = r.BypassPullRequestAllowances.putForm()
		}
		payload["required_pull_request_reviews"] = reviews
	}

	if cur.Restrictions != nil {
		payload["restrictions"] = cur.Restrictions.putForm()
	}

	for key, v := range map[string]*enabledJSON{
		"required_linear_history":          cur.RequiredLinearHistory,
		"allow_force_pushes":               cur.AllowForcePushes,
		"allow_deletions":                  cur.AllowDeletions,
		"block_creations":                  cur.BlockCreations,
		"required_conversation_resolution": cur.RequiredConversationResolution,
		"lock_branch":                      cur.LockBranch,
		"allow_fork_syncing":               cur.AllowForkSyncing,
	} {
		if v != nil {
			payload[key] = v.Enabled
		}
	}
	return payload, nil
}

// buildBranchProtectionUpdatePayload builds the payload to update a protected
// branch. The PUT replaces the whole protection, so it starts from the
// current one and applies only what the manifest sets; everything else,
// including settings gh-infra does not model, keeps its current value.
func buildBranchProtectionUpdatePayload(bp *manifest.BranchProtection, current []byte) (map[string]any, error) {
	payload, err := protectionPutForm(current)
	if err != nil {
		return nil, err
	}

	if bp.EnforceAdmins != nil {
		payload["enforce_admins"] = *bp.EnforceAdmins
	}
	if bp.AllowForcePushes != nil {
		payload["allow_force_pushes"] = *bp.AllowForcePushes
	}
	if bp.AllowDeletions != nil {
		payload["allow_deletions"] = *bp.AllowDeletions
	}

	if bp.RequiredReviews != nil || bp.DismissStaleReviews != nil || bp.RequireCodeOwnerReviews != nil {
		reviews, _ := payload["required_pull_request_reviews"].(map[string]any)
		if reviews == nil {
			reviews = map[string]any{}
		}
		payload["required_pull_request_reviews"] = overlayReviews(reviews, bp)
	}

	if sc := bp.RequireStatusChecks; sc != nil {
		checks := map[string]any{"strict": sc.Strict}
		if sc.Contexts != nil {
			checks["contexts"] = sc.Contexts
		} else if cur, ok := payload["required_status_checks"].(map[string]any); ok {
			checks["checks"] = cur["checks"]
		} else {
			checks["contexts"] = []string{}
		}
		payload["required_status_checks"] = checks
	}

	if bp.RestrictPushes != nil {
		switch {
		case !*bp.RestrictPushes:
			payload["restrictions"] = nil
		case payload["restrictions"] == nil:
			payload["restrictions"] = emptyRestrictions()
		}
	}

	return payload, nil
}
