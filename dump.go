package config

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

const redactedValue = "[REDACTED]"

type DumpOption func(*dumpOptions)

type dumpOptions struct {
	sensitive map[string]struct{}
}

func WithSensitiveKeys(keys ...string) DumpOption {
	return func(options *dumpOptions) {
		for _, key := range keys {
			options.sensitive[strings.ToLower(key)] = struct{}{}
		}
	}
}

func dumpSnapshot(w io.Writer, snapshot *Snapshot, options ...DumpOption) error {
	if snapshot == nil {
		return fmt.Errorf("config: snapshot is nil")
	}
	configuration := dumpOptions{sensitive: make(map[string]struct{})}
	for _, option := range options {
		if option != nil {
			option(&configuration)
		}
	}

	settings := make(map[string]any)
	_ = walkLeaves(nil, snapshot.Settings(), func(path []string, value any) error {
		settings[joinPath(path)] = value
		return nil
	})
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := fmt.Sprint(settings[key])
		if isSensitiveKey(key, configuration.sensitive) {
			value = redactedValue
		}
		if _, err := fmt.Fprintf(w, "%s=%s source=%s\n", key, value, snapshot.Source(key)); err != nil {
			return err
		}
	}
	return nil
}

func isSensitiveKey(key string, exact map[string]struct{}) bool {
	lower := strings.ToLower(key)
	if _, found := exact[lower]; found {
		return true
	}
	for _, part := range strings.FieldsFunc(lower, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	}) {
		switch part {
		case "secret", "password", "passwd", "token", "credential", "privatekey", "apikey", "dsn":
			return true
		}
	}
	return false
}
