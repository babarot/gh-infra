package importer

import "github.com/babarot/gh-infra/internal/manifest"

func FindMatches(parsed *manifest.ParseResult, fullName string) Matches {
	var matches Matches

	for _, repo := range parsed.RepositoryDocs {
		if repo.Resource.Metadata.FullName() != fullName {
			continue
		}
		if repo.FromSet {
			matches.RepositorySets = append(matches.RepositorySets, repo)
			continue
		}
		matches.Repositories = append(matches.Repositories, repo)
	}

	for _, fs := range parsed.FileSetDocs {
		for _, r := range fs.Resource.Spec.Repositories {
			if fs.Resource.RepoFullName(r.Name) == fullName {
				matches.FileSets = append(matches.FileSets, fs)
				break
			}
		}
	}

	return matches
}
