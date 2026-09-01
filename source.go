package config

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Revision identifies a source version. Revisions are opaque to Manager.
type Revision string

// Document is one configuration contribution produced by a Source.
type Document struct {
	Values   map[string]any
	Revision Revision
}

// Event reports that a watched source may have changed.
type Event struct {
	Revision Revision
	Err      error
}

// Source loads one configuration contribution. Sources are merged in the
// order passed to NewManager; later sources have higher priority.
type Source interface {
	Name() string
	Load(context.Context) (Document, error)
}

// WatchSource is a Source that can notify Manager about external changes.
type WatchSource interface {
	Source
	Watch(context.Context, Revision) (<-chan Event, error)
}

type staticSource struct {
	name string
	load func(context.Context) (Document, error)
}

func (s staticSource) Name() string { return s.name }

func (s staticSource) Load(ctx context.Context) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	return s.load(ctx)
}

// YAMLFile returns a required YAML file source.
func YAMLFile(path string) Source {
	return staticSource{name: path, load: func(context.Context) (Document, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return Document{}, fmt.Errorf("config: read %q: %w", path, err)
		}
		values, err := ParseYAML(data)
		if err != nil {
			return Document{}, fmt.Errorf("config: parse %q: %w", path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return Document{}, fmt.Errorf("config: stat %q: %w", path, err)
		}
		return Document{Values: values, Revision: Revision(info.ModTime().UTC().Format(time.RFC3339Nano))}, nil
	}}
}

// OptionalYAMLFile returns a YAML source that contributes no values when the file does not exist.
func OptionalYAMLFile(path string) Source {
	return staticSource{name: path, load: func(context.Context) (Document, error) {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return Document{Values: map[string]any{}}, nil
		}
		if err != nil {
			return Document{}, fmt.Errorf("config: read %q: %w", path, err)
		}
		values, err := ParseYAML(data)
		if err != nil {
			return Document{}, fmt.Errorf("config: parse %q: %w", path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return Document{}, fmt.Errorf("config: stat %q: %w", path, err)
		}
		return Document{Values: values, Revision: Revision(info.ModTime().UTC().Format(time.RFC3339Nano))}, nil
	}}
}

// YAMLBytes returns an in-memory YAML source.
func YAMLBytes(name string, data []byte) Source {
	copyData := append([]byte(nil), data...)
	return staticSource{name: name, load: func(context.Context) (Document, error) {
		values, err := ParseYAML(copyData)
		if err != nil {
			return Document{}, fmt.Errorf("config: parse %q: %w", name, err)
		}
		return Document{Values: values}, nil
	}}
}

// Values returns an in-memory map source.
func Values(name string, values map[string]any) Source {
	copyValues := cloneMap(values)
	return staticSource{name: name, load: func(context.Context) (Document, error) {
		return Document{Values: cloneMap(copyValues)}, nil
	}}
}

// Environment maps variables with the given prefix to dotted configuration keys.
func Environment(prefix string) Source {
	return staticSource{name: "environment", load: func(context.Context) (Document, error) {
		if strings.TrimSpace(prefix) == "" {
			return Document{}, fmt.Errorf("config: environment prefix is required")
		}
		keys := make([]string, 0)
		values := make(map[string]any)
		for _, entry := range os.Environ() {
			key, value, found := strings.Cut(entry, "=")
			if !found || !strings.HasPrefix(key, prefix) {
				continue
			}
			path := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(key, prefix), "_", "."))
			if path == "" {
				continue
			}
			keys = append(keys, path)
			setPath(values, path, value)
		}
		sort.Strings(keys)
		return Document{Values: values, Revision: Revision(strings.Join(keys, ","))}, nil
	}}
}

// ParseYAML parses exactly one YAML document into a string-keyed map.
func ParseYAML(data []byte) (map[string]any, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	if values == nil {
		values = make(map[string]any)
	}
	return normalizeMap(values), nil
}
