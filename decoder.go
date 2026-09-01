package config

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

// Validator validates a fully decoded configuration before publication.
type Validator interface{ Validate() error }

func decode[T any](defaults T, values map[string]any) (T, error) {
	result := deepClone(defaults)
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			textUnmarshallerHook(),
		),
		ErrorUnused: true, MatchName: strings.EqualFold, Result: &result,
		Squash: true, TagName: "yaml", WeaklyTypedInput: true,
	})
	if err != nil {
		var zero T
		return zero, fmt.Errorf("config: create decoder: %w", err)
	}
	if err := decoder.Decode(values); err != nil {
		var zero T
		return zero, fmt.Errorf("config: decode: %w", err)
	}
	if validator, ok := any(&result).(Validator); ok {
		if err := validator.Validate(); err != nil {
			var zero T
			return zero, fmt.Errorf("config: validate: %w", err)
		}
	} else if validator, ok := any(result).(Validator); ok {
		if err := validator.Validate(); err != nil {
			var zero T
			return zero, fmt.Errorf("config: validate: %w", err)
		}
	}
	return result, nil
}

func textUnmarshallerHook() mapstructure.DecodeHookFuncType {
	textType := reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	return func(from, to reflect.Type, data any) (any, error) {
		if from.Kind() != reflect.String || !reflect.PointerTo(to).Implements(textType) {
			return data, nil
		}
		value := reflect.New(to)
		if err := value.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(data.(string))); err != nil {
			return nil, err
		}
		return value.Elem().Interface(), nil
	}
}

func deepClone[T any](value T) T {
	cloned := cloneReflect(reflect.ValueOf(value))
	if !cloned.IsValid() {
		var zero T
		return zero
	}
	return cloned.Interface().(T)
}

func cloneReflect(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type()).Elem()
		result.Set(cloneReflect(value.Elem()))
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(cloneReflect(value.Elem()))
		return result
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			result.SetMapIndex(cloneReflect(iter.Key()), cloneReflect(iter.Value()))
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneReflect(value.Index(i)))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneReflect(value.Index(i)))
		}
		return result
	case reflect.Struct:
		if value.Type() == reflect.TypeOf(time.Time{}) {
			return value
		}
		result := reflect.New(value.Type()).Elem()
		for i := 0; i < value.NumField(); i++ {
			if result.Field(i).CanSet() && value.Field(i).CanInterface() {
				result.Field(i).Set(cloneReflect(value.Field(i)))
			}
		}
		return result
	default:
		return value
	}
}
