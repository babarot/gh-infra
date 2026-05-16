package manifest

// RepositorySet represents multiple repositories with shared defaults.
type RepositorySet struct {
	APIVersion   string                 `yaml:"apiVersion"`
	Kind         string                 `yaml:"kind"`
	Metadata     RepositorySetMetadata  `yaml:"metadata"`
	Defaults     *RepositorySetDefaults `yaml:"defaults,omitempty"`
	Repositories []RepositorySetEntry   `yaml:"repositories"`
}

type RepositorySetMetadata struct {
	Owner string `yaml:"owner"`
}

type RepositorySetDefaults struct {
	Reconcile *RepositoryReconcile `yaml:"reconcile,omitempty"`
	Spec      RepositorySpec       `yaml:"spec"`
}

// RepositoryCondition gates settings on a runtime-evaluated repo property.
// All specified fields must match for the condition to be satisfied.
// Currently only Visibility is supported.
type RepositoryCondition struct {
	Visibility string `yaml:"visibility"`
}

type RepositorySetEntry struct {
	Name            string               `yaml:"name"`
	Reconcile       *RepositoryReconcile `yaml:"reconcile,omitempty"`
	Spec            RepositorySpec       `yaml:"spec"`
	When            *RepositoryCondition `yaml:"when,omitempty"`
	ConditionalSpec *RepositorySpec      `yaml:"conditional_spec,omitempty"`
}
