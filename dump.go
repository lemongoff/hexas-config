package config

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// DumpOption customizes safe diagnostic output.
type DumpOption func(*dumpOptions)

type dumpOptions struct{ sensitive map[string]struct{} }

// WithSensitiveKeys adds exact dotted keys to the default redaction set.
func WithSensitiveKeys(keys ...string) DumpOption {
	return func(options *dumpOptions) {
		for _, key := range keys {
			options.sensitive[normalizeKey(key)] = struct{}{}
		}
	}
}

// DumpTo writes sorted and redacted configuration diagnostics.
func (s Snapshot[T]) DumpTo(writer io.Writer, options ...DumpOption) error {
	configuration := dumpOptions{sensitive: make(map[string]struct{})}
	for _, option := range options {
		if option != nil {
			option(&configuration)
		}
	}
	values := flatten(structMap(s.value))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := fmt.Sprint(values[key])
		if sensitiveKey(key, configuration.sensitive) {
			value = "[REDACTED]"
		}
		if _, err := fmt.Fprintf(writer, "%s=%s\n", key, value); err != nil {
			return err
		}
	}
	return nil
}

func sensitiveKey(key string, exact map[string]struct{}) bool {
	if _, ok := exact[normalizeKey(key)]; ok {
		return true
	}
	for _, part := range strings.FieldsFunc(strings.ToLower(key), func(r rune) bool { return r == '.' || r == '_' || r == '-' }) {
		switch part {
		case "secret", "password", "passwd", "token", "credential", "privatekey", "apikey", "dsn", "pass":
			return true
		}
	}
	return false
}
