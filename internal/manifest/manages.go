package manifest

// These report which settings a repository spec manages, so plan fetches
// only those from GitHub (and needs only their permissions). They match what
// the repository diff and dependency checks read: a setting a spec does not
// manage is never compared.

// ManagesBranchProtection reports whether branch protection is managed.
func (s RepositorySpec) ManagesBranchProtection() bool {
	return s.BranchProtectionSet || len(s.BranchProtection) > 0
}

// ManagesRulesets reports whether rulesets are managed.
func (s RepositorySpec) ManagesRulesets() bool {
	return s.RulesetsSet || len(s.Rulesets) > 0
}

// ManagesSecrets reports whether any secret is declared. `secrets: []` only
// opts out of the defaults' secrets and needs no fetch.
func (s RepositorySpec) ManagesSecrets() bool { return len(s.Secrets) > 0 }

// ManagesVariables reports whether any variable is declared.
func (s RepositorySpec) ManagesVariables() bool { return len(s.Variables) > 0 }

// ManagesLabels reports whether labels are managed.
func (s RepositorySpec) ManagesLabels() bool {
	return s.LabelsSet || len(s.Labels) > 0 || s.LabelSync != nil
}

// ManagesMilestones reports whether any milestone is declared.
func (s RepositorySpec) ManagesMilestones() bool { return len(s.Milestones) > 0 }

// ManagesActions reports whether Actions settings are managed.
func (s RepositorySpec) ManagesActions() bool { return s.Actions != nil }

// ManagesCommitMessageSettings reports whether any setting read from
// GET /repos/{owner}/{repo} (rather than gh repo view) is managed: auto-merge,
// the merge and squash commit title and message, and pull requests.
func (s RepositorySpec) ManagesCommitMessageSettings() bool {
	if m := s.MergeStrategy; m != nil {
		if m.AllowAutoMerge != nil || m.MergeCommitTitle != nil || m.MergeCommitMessage != nil ||
			m.SquashMergeCommitTitle != nil || m.SquashMergeCommitMessage != nil {
			return true
		}
	}
	return s.Features != nil && s.Features.PullRequests != nil
}

// ManagesReleaseImmutability reports whether release immutability is managed.
func (s RepositorySpec) ManagesReleaseImmutability() bool { return s.ReleaseImmutability != nil }

// ManagesVulnerabilityAlerts reports whether the current vulnerability alerts
// setting is needed: when it is managed, and when automated security fixes
// are, since enabling those requires alerts to be enabled.
func (s RepositorySpec) ManagesVulnerabilityAlerts() bool {
	return s.Security != nil && (s.Security.VulnerabilityAlerts != nil || s.Security.AutomatedSecurityFixes != nil)
}

// ManagesAutomatedSecurityFixes reports whether automated security fixes are managed.
func (s RepositorySpec) ManagesAutomatedSecurityFixes() bool {
	return s.Security != nil && s.Security.AutomatedSecurityFixes != nil
}

// ManagesPrivateVulnerabilityReporting reports whether private vulnerability
// reporting is managed.
func (s RepositorySpec) ManagesPrivateVulnerabilityReporting() bool {
	return s.Security != nil && s.Security.PrivateVulnerabilityReporting != nil
}
