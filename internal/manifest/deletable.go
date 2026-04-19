package manifest

import (
	"bytes"

	"github.com/goccy/go-yaml"
)

// Deletable is a three-state wrapper for YAML fields that support explicit deletion.
//
//   - Zero value (field omitted from YAML) → IsSet()=false, IsDelete()=false
//   - Explicit null in YAML               → IsSet()=true,  IsDelete()=true
//   - Value present in YAML               → IsSet()=true,  IsDelete()=false
type Deletable[T any] struct {
	value    T
	isSet    bool
	isDelete bool
}

// IsSet reports whether the field appeared in the YAML document (including null).
func (n Deletable[T]) IsSet() bool { return n.isSet }

// IsDelete reports whether the field was explicitly set to null.
func (n Deletable[T]) IsDelete() bool { return n.isSet && n.isDelete }

// HasValue reports whether the field has a concrete value.
func (n Deletable[T]) HasValue() bool { return n.isSet && !n.isDelete }

// Get returns the wrapped value.
func (n Deletable[T]) Get() T { return n.value }

// GetOK returns the wrapped value and whether it is a concrete value.
func (n Deletable[T]) GetOK() (T, bool) {
	return n.value, n.HasValue()
}

// NewDeletable creates a Deletable with a value (isSet=true, isDelete=false).
func NewDeletable[T any](v T) Deletable[T] {
	return Deletable[T]{value: v, isSet: true}
}

// DeleteValue creates a null-marked Deletable (isSet=true, isDelete=true).
func DeleteValue[T any]() Deletable[T] {
	return Deletable[T]{isSet: true, isDelete: true}
}

// HasItems reports whether a deletable slice has one or more concrete items.
func HasItems[T any](d Deletable[[]T]) bool {
	return d.HasValue() && len(d.value) > 0
}

// MergeDeletableSlice applies RepositorySet-style merge semantics to a
// deletable slice: delete overrides everything, non-empty values merge, and
// omitted or empty overrides leave the base unchanged.
func MergeDeletableSlice[T any](
	base Deletable[[]T],
	override Deletable[[]T],
	merge func([]T, []T) []T,
) Deletable[[]T] {
	if override.IsDelete() {
		return DeleteValue[[]T]()
	}
	if HasItems(override) {
		return NewDeletable(merge(base.Get(), override.Get()))
	}
	return base
}

type deletableMarker interface {
	markDelete()
}

func (n *Deletable[T]) markDelete() {
	var zero T
	n.value = zero
	n.isSet = true
	n.isDelete = true
}

// UnmarshalYAML implements BytesUnmarshaler.
// Parser code separately marks explicit null values for the fields that use
// Deletable, because upstream go-yaml does not call unmarshaler hooks for null.
func (n *Deletable[T]) UnmarshalYAML(data []byte) error {
	n.isSet = true
	n.isDelete = false
	if isYAMLNull(data) {
		n.markDelete()
		return nil
	}
	return yaml.Unmarshal(data, &n.value)
}

// MarshalYAML serializes the Deletable value to YAML.
//   - IsDelete → nil (outputs "null")
//   - Has value → outputs the inner value directly
//   - Unset → IsZero()=true causes omitempty to drop the field
func (n Deletable[T]) MarshalYAML() (any, error) {
	if n.isDelete {
		return nil, nil
	}
	return n.value, nil
}

// IsZero supports the omitempty tag. Returns true when the field was not set,
// causing the YAML encoder to omit it.
func (n Deletable[T]) IsZero() bool { return !n.isSet }

func isYAMLNull(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) == 0 || string(trimmed) == "null" || string(trimmed) == "~"
}
