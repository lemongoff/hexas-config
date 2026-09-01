package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/lemongoff/hexas-config/internal/resolver"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
)

var placeholderPattern = regexp.MustCompile(`\$\{([\w.]+)\}`)

func loadSnapshot(layout Layout, overrides map[string]overrideValue) (*Snapshot, error) {
	snapshot := newSnapshot()
	if err := snapshot.readLayout(layout, overrides); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (snapshot *Snapshot) readLayout(layout Layout, overrides map[string]overrideValue) error {
	templateExists, err := regularFile(layout.TemplateFile)
	if err != nil {
		return fmt.Errorf("config: inspect template %q: %w", layout.TemplateFile, err)
	}
	if !templateExists {
		return fmt.Errorf("config: template %q does not exist", layout.TemplateFile)
	}

	snapshot.values.SetConfigFile(layout.TemplateFile)
	if err := snapshot.values.ReadInConfig(); err != nil {
		return fmt.Errorf("config: read template: %w", err)
	}
	snapshot.metadata.TemplateFile = layout.TemplateFile

	overlayExists, err := regularFile(layout.OverlayFile)
	if err != nil {
		return fmt.Errorf("config: inspect overlay %q: %w", layout.OverlayFile, err)
	}
	if overlayExists {
		overlay := viper.New()
		overlay.SetConfigFile(layout.OverlayFile)
		if err := overlay.ReadInConfig(); err != nil {
			return fmt.Errorf("config: read overlay: %w", err)
		}
		if err := snapshot.merge(overlay.AllSettings(), "overlay"); err != nil {
			return fmt.Errorf("config: merge overlay: %w", err)
		}
		snapshot.metadata.OverlayFile = layout.OverlayFile
	}

	resolvers := []resolver.Resolver{resolver.NewEnvironment()}
	if layout.EnvFile != "" {
		resolvers = append(resolvers, resolver.NewFile(layout.EnvFile, "env"))
	}
	for _, file := range layout.ReplacementFiles {
		resolvers = append(resolvers, resolver.NewFile(file.Path, file.Type))
	}
	resolvers = append(resolvers, resolver.NewDynamic())
	for _, current := range resolvers {
		if err := current.Init(); err != nil {
			return fmt.Errorf("config: initialize placeholder source %q: %w", current.Name(), err)
		}
		snapshot.metadata.PlaceholderSources = append(snapshot.metadata.PlaceholderSources, current.Name())
	}
	if err := snapshot.resolvePlaceholders(snapshot.values.AllSettings(), resolvers); err != nil {
		return err
	}

	usedOverrides, err := snapshot.applyOverrides(snapshot.values.AllSettings(), overrides)
	if err != nil {
		return err
	}
	snapshot.metadata.OverrideSources = usedOverrides
	return nil
}

func (snapshot *Snapshot) merge(values map[string]any, source string) error {
	return walkLeaves(nil, values, func(path []string, value any) error {
		snapshot.setWithSource(joinPath(path), value, source)
		return nil
	})
}

func (snapshot *Snapshot) applyOverrides(values map[string]any, overrides map[string]overrideValue) ([]string, error) {
	used := make(map[string]struct{})
	err := walkLeaves(nil, values, func(path []string, _ any) error {
		key := joinPath(path)
		environmentKey := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		if value, found := os.LookupEnv(environmentKey); found {
			snapshot.setWithSource(key, value, "environment")
			used["environment"] = struct{}{}
		}
		if override, found := overrides[normalizeKey(key)]; found {
			snapshot.setWithSource(key, override.value, override.source)
			used[override.source] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sources := make([]string, 0, len(used))
	for source := range used {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources, nil
}

func (snapshot *Snapshot) resolvePlaceholders(values map[string]any, resolvers []resolver.Resolver) error {
	return walkLeaves(nil, values, func(path []string, value any) error {
		text, ok := value.(string)
		if !ok || !placeholderPattern.MatchString(text) {
			return nil
		}
		resolved, source, err := resolvePlaceholderValue(text, resolvers, make(map[string]struct{}), 0)
		if err != nil {
			return fmt.Errorf("config: resolve placeholder for %q: %w", joinPath(path), err)
		}
		snapshot.setWithSource(joinPath(path), resolved, source)
		return nil
	})
}

func resolvePlaceholderValue(value string, resolvers []resolver.Resolver, seen map[string]struct{}, depth int) (any, string, error) {
	if depth >= 64 {
		return nil, "", fmt.Errorf("placeholder expansion exceeded 64 steps")
	}
	if _, found := seen[value]; found {
		return nil, "", fmt.Errorf("placeholder cycle detected")
	}
	seen[value] = struct{}{}
	defer delete(seen, value)

	var result any = value
	var source string
	for _, match := range placeholderPattern.FindAllString(value, -1) {
		resolved, resolvedSource, err := findPlaceholder(match, resolvers)
		if err != nil {
			return nil, "", err
		}
		source = resolvedSource
		if value == match {
			result = resolved
		} else {
			value = strings.ReplaceAll(value, match, cast.ToString(resolved))
			result = value
		}
	}

	if text, ok := result.(string); ok && placeholderPattern.MatchString(text) {
		return resolvePlaceholderValue(text, resolvers, seen, depth+1)
	}
	return result, source, nil
}

func findPlaceholder(key string, resolvers []resolver.Resolver) (any, string, error) {
	for _, current := range resolvers {
		if value, found := current.Find(key); found {
			return value, current.Name(), nil
		}
	}
	return nil, "", fmt.Errorf("placeholder %s is not defined", key)
}

func joinPath(path []string) string {
	return strings.Join(path, ".")
}

func walkLeaves(prefix []string, values map[string]any, visit func([]string, any) error) error {
	for key, value := range values {
		path := append(append([]string(nil), prefix...), key)
		if nested, ok := value.(map[string]any); ok {
			if err := walkLeaves(path, nested, visit); err != nil {
				return err
			}
			continue
		}
		if err := visit(path, value); err != nil {
			return err
		}
	}
	return nil
}

func regularFile(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("path is a directory")
		}
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
