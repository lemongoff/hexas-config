package watch

import (
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	watchers map[string]map[uint64]func(string, string)
	nextID   uint64
}

func NewRegistry() *Registry {
	return &Registry{watchers: make(map[string]map[uint64]func(string, string))}
}

func (r *Registry) Subscribe(keys []string, callback func(string, string)) func() {
	if callback == nil {
		return func() {}
	}
	keys = normalizeKeys(keys)
	if len(keys) == 0 {
		return func() {}
	}

	r.mu.Lock()
	r.nextID++
	id := r.nextID
	for _, key := range keys {
		if r.watchers[key] == nil {
			r.watchers[key] = make(map[uint64]func(string, string))
		}
		r.watchers[key][id] = callback
	}
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			for _, key := range keys {
				delete(r.watchers[key], id)
				if len(r.watchers[key]) == 0 {
					delete(r.watchers, key)
				}
			}
		})
	}
}

func (r *Registry) Notify(key, value string) {
	key = normalizeKey(key)
	r.mu.RLock()
	callbacks := copyCallbacks(r.watchers[key])
	r.mu.RUnlock()

	for _, callback := range callbacks {
		callback(key, value)
	}
}

func (r *Registry) NotifyAll(valueFor func(string) string) {
	if valueFor == nil {
		return
	}
	r.mu.RLock()
	snapshot := make(map[string][]func(string, string), len(r.watchers))
	for key, callbacks := range r.watchers {
		snapshot[key] = copyCallbacks(callbacks)
	}
	r.mu.RUnlock()

	for key, callbacks := range snapshot {
		value := valueFor(key)
		for _, callback := range callbacks {
			callback(key, value)
		}
	}
}

func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]string, 0, len(r.watchers))
	for key := range r.watchers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		key = normalizeKey(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func copyCallbacks(source map[uint64]func(string, string)) []func(string, string) {
	result := make([]func(string, string), 0, len(source))
	for _, callback := range source {
		result = append(result, callback)
	}
	return result
}
