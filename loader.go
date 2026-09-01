package config

import (
	"fmt"
	"reflect"
	"strings"
)

type LoaderOption interface {
	apply(*loaderOptions) error
}

type loaderOptionFunc func(*loaderOptions) error

func (option loaderOptionFunc) apply(options *loaderOptions) error {
	return option(options)
}

type loaderOptions struct {
	overrides map[string]overrideValue
}

type overrideValue struct {
	value  any
	source string
}

type Loader struct {
	layout    Layout
	overrides map[string]overrideValue
}

func NewLoader(layout Layout, options ...LoaderOption) (*Loader, error) {
	if err := layout.validate(); err != nil {
		return nil, err
	}
	configuration := loaderOptions{overrides: make(map[string]overrideValue)}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option.apply(&configuration); err != nil {
			return nil, err
		}
	}
	return &Loader{
		layout:    cloneLayout(layout),
		overrides: configuration.overrides,
	}, nil
}

func WithOverrides(source string, values map[string]any) LoaderOption {
	normalizedSource := strings.TrimSpace(source)
	return loaderOptionFunc(func(options *loaderOptions) error {
		if normalizedSource == "" {
			return fmt.Errorf("config: override source is required")
		}
		for key, value := range values {
			key = normalizeKey(key)
			if key == "" {
				return fmt.Errorf("config: override key is required")
			}
			options.overrides[key] = overrideValue{value: cloneValue(value), source: normalizedSource}
		}
		return nil
	})
}

func (loader *Loader) Load() (*Snapshot, error) {
	if loader == nil {
		return nil, fmt.Errorf("config: loader is nil")
	}
	return loadSnapshot(loader.layout, loader.overrides)
}

func (loader *Loader) LoadInto(target any) (*Snapshot, error) {
	snapshot, err := loader.Load()
	if err != nil {
		return nil, err
	}
	if err := decodeAndValidate(snapshot, target); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func decodeAndValidate(snapshot *Snapshot, target any) error {
	if target == nil || isNil(target) {
		return fmt.Errorf("config: decode target is nil")
	}
	if err := snapshot.Unmarshal(target); err != nil {
		return fmt.Errorf("decode configuration: %w", err)
	}
	if validator, ok := target.(Validator); ok {
		if err := validator.Validate(); err != nil {
			return fmt.Errorf("validate configuration: %w", err)
		}
	}
	return nil
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
