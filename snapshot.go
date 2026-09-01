package config

import (
	"io"
	"strings"

	"github.com/spf13/viper"
)

type Validator interface {
	Validate() error
}

type Snapshot struct {
	values   *viper.Viper
	sources  map[string]string
	metadata Metadata
}

func newSnapshot() *Snapshot {
	return &Snapshot{
		values:  viper.New(),
		sources: make(map[string]string),
	}
}

func (snapshot *Snapshot) clone() *Snapshot {
	result := newSnapshot()
	copySettings(result, snapshot.values.AllSettings())
	result.sources = cloneStrings(snapshot.sources)
	result.metadata = snapshot.metadata.clone()
	return result
}

func copySettings(target *Snapshot, settings map[string]any) {
	_ = walkLeaves(nil, settings, func(path []string, value any) error {
		target.values.Set(joinPath(path), cloneValue(value))
		return nil
	})
}

func (snapshot *Snapshot) setWithSource(key string, value any, source string) {
	key = normalizeKey(key)
	snapshot.values.Set(key, cloneValue(value))
	snapshot.sources[key] = source
}

func (snapshot *Snapshot) Unmarshal(target any) error {
	return snapshot.values.Unmarshal(target)
}

func (snapshot *Snapshot) Settings() map[string]any {
	return cloneMap(snapshot.values.AllSettings())
}

func (snapshot *Snapshot) Metadata() Metadata {
	return snapshot.metadata.clone()
}

func (snapshot *Snapshot) Source(key string) string {
	key = normalizeKey(key)
	if source := snapshot.sources[key]; source != "" {
		return source
	}
	if snapshot.values.IsSet(key) {
		return "template"
	}
	return ""
}

func (snapshot *Snapshot) GetString(key string) string {
	return snapshot.values.GetString(key)
}

func (snapshot *Snapshot) GetBool(key string) bool {
	return snapshot.values.GetBool(key)
}

func (snapshot *Snapshot) GetInt(key string) int {
	return snapshot.values.GetInt(key)
}

func (snapshot *Snapshot) GetInt32(key string) int32 {
	return snapshot.values.GetInt32(key)
}

func (snapshot *Snapshot) GetInt64(key string) int64 {
	return snapshot.values.GetInt64(key)
}

func (snapshot *Snapshot) GetUint32(key string) uint32 {
	return snapshot.values.GetUint32(key)
}

func (snapshot *Snapshot) GetUint64(key string) uint64 {
	return snapshot.values.GetUint64(key)
}

func (snapshot *Snapshot) GetFloat64(key string) float64 {
	return snapshot.values.GetFloat64(key)
}

func (snapshot *Snapshot) GetStringSlice(key string) []string {
	return append([]string(nil), snapshot.values.GetStringSlice(key)...)
}

func (snapshot *Snapshot) GetStringMap(key string) map[string]any {
	return cloneMap(snapshot.values.GetStringMap(key))
}

func (snapshot *Snapshot) IsSet(key string) bool {
	return snapshot.values.IsSet(key)
}

func (snapshot *Snapshot) DumpTo(writer io.Writer, options ...DumpOption) error {
	return dumpSnapshot(writer, snapshot, options...)
}

func cloneMap(values map[string]any) map[string]any {
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = cloneValue(value)
	}
	return result
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = cloneValue(item)
		}
		return result
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func cloneStrings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
