package manifest

import "fmt"

func validateRepository(repo *Repository, source string) error {
	if repo.Metadata.Name == "" {
		return fmt.Errorf("%s: metadata.name is required", source)
	}
	if repo.Metadata.Owner == "" {
		return fmt.Errorf("%s: metadata.owner is required for %q", source, repo.Metadata.Name)
	}
	if repo.Spec.Visibility != nil {
		switch *repo.Spec.Visibility {
		case "public", "private", "internal":
		default:
			return fmt.Errorf("%s: invalid visibility %q for %q", source, *repo.Spec.Visibility, repo.Metadata.Name)
		}
	}
	for _, bp := range repo.Spec.BranchProtection {
		if bp.Pattern == "" {
			return fmt.Errorf("%s: branch_protection.pattern is required for %q", source, repo.Metadata.Name)
		}
	}
	return nil
}
