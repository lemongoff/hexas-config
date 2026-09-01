package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mitchellh/mapstructure"
)

func normalizeKey(key string) string { return strings.ToLower(strings.TrimSpace(key)) }

func normalizeMap(values map[string]any) map[string]any {
	result := make(map[string]any, len(values))
	for key, value := range values {
		key = normalizeKey(key)
		switch typed := value.(type) {
		case map[string]any:
			result[key] = normalizeMap(typed)
		case map[string]string:
			nested := make(map[string]any, len(typed))
			for nestedKey, nestedValue := range typed {
				nested[nestedKey] = nestedValue
			}
			result[key] = normalizeMap(nested)
		case map[any]any:
			nested := make(map[string]any, len(typed))
			for nestedKey, nestedValue := range typed {
				nested[normalizeKey(fmt.Sprint(nestedKey))] = nestedValue
			}
			result[key] = normalizeMap(nested)
		default:
			result[key] = cloneAny(value)
		}
	}
	return result
}

func mergeMap(target, source map[string]any) {
	for key, value := range source {
		if sourceMap, ok := value.(map[string]any); ok {
			targetMap, _ := target[key].(map[string]any)
			if targetMap == nil {
				targetMap = make(map[string]any)
				target[key] = targetMap
			}
			mergeMap(targetMap, sourceMap)
			continue
		}
		target[key] = cloneAny(value)
	}
}

func setPath(values map[string]any, path string, value any) {
	parts := strings.Split(normalizeKey(path), ".")
	current := values
	for _, part := range parts[:len(parts)-1] {
		next, _ := current[part].(map[string]any)
		if next == nil {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = cloneAny(value)
}

func cloneMap(values map[string]any) map[string]any {
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = cloneAny(value)
	}
	return result
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case map[string]string:
		result := make(map[string]string, len(typed))
		for key, item := range typed {
			result[key] = item
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = cloneAny(item)
		}
		return result
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func structMap(value any) map[string]any {
	result := make(map[string]any)
	_ = mapstructure.Decode(value, &result)
	return normalizeMap(result)
}

func flatten(values map[string]any) map[string]any {
	result := make(map[string]any)
	var visit func(string, map[string]any)
	visit = func(prefix string, current map[string]any) {
		for key, value := range current {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			if nested, ok := value.(map[string]any); ok {
				visit(path, nested)
			} else {
				result[path] = value
			}
		}
	}
	visit("", values)
	return result
}

func flattenKeys(values map[string]any) []string {
	flat := flatten(values)
	keys := make([]string, 0, len(flat))
	for key := range flat {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
