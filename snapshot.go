package config

import "time"

// SourceMetadata records one source contribution without retaining values.
type SourceMetadata struct {
	Name     string
	Revision Revision
}

// Metadata describes a published configuration snapshot.
type Metadata struct {
	LoadedAt time.Time
	Checksum string
	Sources  []SourceMetadata
}

// Snapshot is an immutable, typed configuration publication.
type Snapshot[T any] struct {
	value    T
	metadata Metadata
}

// Value returns a detached copy of the typed configuration.
func (s Snapshot[T]) Value() T { return deepClone(s.value) }

// Metadata returns detached publication metadata.
func (s Snapshot[T]) Metadata() Metadata {
	result := s.metadata
	result.Sources = append([]SourceMetadata(nil), s.metadata.Sources...)
	return result
}
