package manifest

import (
	"bytes"

	"github.com/goccy/go-yaml"
)

// Nullable is a three-state wrapper for YAML fields that support explicit deletion.
//
//   - Zero value (field omitted from YAML) → IsSet()=false, IsNull()=false
//   - Explicit null in YAML               → IsSet()=true,  IsNull()=true
//   - Value present in YAML               → IsSet()=true,  IsNull()=false
type Nullable[T any] struct {
	Value  T
	isSet  bool
	isNull bool
}

// IsSet reports whether the field appeared in the YAML document (including null).
func (n Nullable[T]) IsSet() bool { return n.isSet }

// IsNull reports whether the field was explicitly set to null.
func (n Nullable[T]) IsNull() bool { return n.isSet && n.isNull }

// NewNullable creates a Nullable with a value (isSet=true, isNull=false).
func NewNullable[T any](v T) Nullable[T] {
	return Nullable[T]{Value: v, isSet: true}
}

// NullValue creates a null-marked Nullable (isSet=true, isNull=true).
func NullValue[T any]() Nullable[T] {
	return Nullable[T]{isSet: true, isNull: true}
}

// UnmarshalYAML implements BytesUnmarshaler.
// With the forked go-yaml, this method is called even when the YAML value is null.
//   - Called → isSet=true (field exists in YAML)
//   - data is "null" → isNull=true
//   - Otherwise → decode into Value
func (n *Nullable[T]) UnmarshalYAML(data []byte) error {
	n.isSet = true
	if isYAMLNull(data) {
		n.isNull = true
		return nil
	}
	return yaml.Unmarshal(data, &n.Value)
}

// MarshalYAML serializes the Nullable value to YAML.
//   - IsNull → nil (outputs "null")
//   - Has value → outputs the inner value directly
//   - Unset → IsZero()=true causes omitempty to drop the field
func (n Nullable[T]) MarshalYAML() (any, error) {
	if n.isNull {
		return nil, nil
	}
	return n.Value, nil
}

// IsZero supports the omitempty tag. Returns true when the field was not set,
// causing the YAML encoder to omit it.
func (n Nullable[T]) IsZero() bool { return !n.isSet }

func isYAMLNull(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) == 0 || string(trimmed) == "null" || string(trimmed) == "~"
}
