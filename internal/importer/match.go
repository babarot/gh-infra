package importer

import "github.com/babarot/gh-infra/internal/manifest"

// FindMatches searches parsed manifests for resources matching the given full name (owner/repo).
func FindMatches(parsed *manifest.ParseResult, fullName string) Matches {
	var m Matches

	for _, doc := range parsed.RepositoryDocs {
		if doc.Resource.Metadata.FullName() != fullName {
			continue
		}
		if doc.FromSet {
			m.RepositorySets = append(m.RepositorySets, doc)
		} else {
			m.Repositories = append(m.Repositories, doc)
		}
	}

	for _, doc := range parsed.FileDocs {
		for _, repo := range doc.Resource.Spec.Repositories {
			if doc.Resource.RepoFullName(repo.Name) == fullName {
				m.FileSets = append(m.FileSets, doc)
				break
			}
		}
	}

	return m
}
