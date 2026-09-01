package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Change describes one successful typed configuration publication.
type Change[T any] struct {
	Previous *Snapshot[T]
	Current  Snapshot[T]
	Changed  []string
}

// Watcher observes successful publications after the atomic swap and outside Manager locks.
type Watcher[T any] func(context.Context, Change[T]) error

// Manager loads ordered sources, validates a new typed value, and publishes it atomically.
type Manager[T any] struct {
	defaults T
	sources  []Source

	reloadMu sync.Mutex
	memoryMu sync.RWMutex
	memory   map[string]any
	current  atomic.Pointer[Snapshot[T]]

	watchMu  sync.RWMutex
	nextID   uint64
	watchers map[uint64]Watcher[T]
}

// NewManager constructs an isolated typed configuration manager.
func NewManager[T any](defaults T, sources ...Source) (*Manager[T], error) {
	if len(sources) == 0 {
		return nil, errors.New("config: at least one source is required")
	}
	for i, source := range sources {
		if source == nil || strings.TrimSpace(source.Name()) == "" {
			return nil, fmt.Errorf("config: source %d is invalid", i)
		}
	}
	return &Manager[T]{defaults: deepClone(defaults), sources: append([]Source(nil), sources...), memory: make(map[string]any), watchers: make(map[uint64]Watcher[T])}, nil
}

// Load builds and publishes a complete configuration snapshot.
func (m *Manager[T]) Load(ctx context.Context) error {
	if m == nil {
		return errors.New("config: manager is nil")
	}
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	values := make(map[string]any)
	metadata := Metadata{LoadedAt: time.Now().UTC()}
	for _, source := range m.sources {
		document, err := source.Load(ctx)
		if err != nil {
			return fmt.Errorf("config: load source %q: %w", source.Name(), err)
		}
		mergeMap(values, normalizeMap(document.Values))
		metadata.Sources = append(metadata.Sources, SourceMetadata{Name: source.Name(), Revision: document.Revision})
	}
	m.memoryMu.RLock()
	memory := cloneMap(m.memory)
	m.memoryMu.RUnlock()
	mergeMap(values, memory)

	typed, err := decode(m.defaults, values)
	if err != nil {
		return err
	}
	metadata.Checksum, err = checksum(values)
	if err != nil {
		return err
	}

	next := &Snapshot[T]{value: typed, metadata: metadata}
	previous := m.current.Swap(next)
	m.notify(ctx, previous, *next)
	return nil
}

// Current returns the current immutable snapshot, if one has been published.
func (m *Manager[T]) Current() (Snapshot[T], bool) {
	if m == nil {
		return Snapshot[T]{}, false
	}
	current := m.current.Load()
	if current == nil {
		return Snapshot[T]{}, false
	}
	return Snapshot[T]{value: deepClone(current.value), metadata: current.Metadata()}, true
}

// SetMemory sets a temporary highest-priority override. Call Load to validate and publish it.
func (m *Manager[T]) SetMemory(key string, value any) error {
	if m == nil {
		return errors.New("config: manager is nil")
	}
	key = normalizeKey(key)
	if key == "" {
		return errors.New("config: memory key is required")
	}
	m.memoryMu.Lock()
	setPath(m.memory, key, cloneAny(value))
	m.memoryMu.Unlock()
	return nil
}

// ClearMemory clears every temporary override. Call Load to publish the result.
func (m *Manager[T]) ClearMemory() {
	if m == nil {
		return
	}
	m.memoryMu.Lock()
	m.memory = make(map[string]any)
	m.memoryMu.Unlock()
}

// Subscribe registers a watcher and returns an idempotent cancellation function.
func (m *Manager[T]) Subscribe(watcher Watcher[T]) func() {
	if m == nil || watcher == nil {
		return func() {}
	}
	m.watchMu.Lock()
	m.nextID++
	id := m.nextID
	m.watchers[id] = watcher
	m.watchMu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { m.watchMu.Lock(); delete(m.watchers, id); m.watchMu.Unlock() }) }
}

// Watch starts every WatchSource. A source event triggers a complete reload.
func (m *Manager[T]) Watch(ctx context.Context) <-chan error {
	errs := make(chan error, len(m.sources))
	var group sync.WaitGroup
	for _, source := range m.sources {
		watched, ok := source.(WatchSource)
		if !ok {
			continue
		}
		group.Add(1)
		go func(source WatchSource) {
			defer group.Done()
			var revision Revision
			if current, ok := m.Current(); ok {
				for _, item := range current.metadata.Sources {
					if item.Name == source.Name() {
						revision = item.Revision
					}
				}
			}
			events, err := source.Watch(ctx, revision)
			if err != nil {
				sendError(ctx, errs, fmt.Errorf("config: watch source %q: %w", source.Name(), err))
				return
			}
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-events:
					if !ok {
						return
					}
					if event.Err != nil {
						sendError(ctx, errs, fmt.Errorf("config: watch source %q: %w", source.Name(), event.Err))
						continue
					}
					if err := m.Load(ctx); err != nil {
						sendError(ctx, errs, err)
					}
				}
			}
		}(watched)
	}
	go func() { group.Wait(); close(errs) }()
	return errs
}

func (m *Manager[T]) notify(ctx context.Context, previous *Snapshot[T], current Snapshot[T]) {
	m.watchMu.RLock()
	watchers := make([]Watcher[T], 0, len(m.watchers))
	for _, watcher := range m.watchers {
		watchers = append(watchers, watcher)
	}
	m.watchMu.RUnlock()
	change := Change[T]{Current: current, Changed: changedKeys(previous, current)}
	if previous != nil {
		copyPrevious := Snapshot[T]{value: deepClone(previous.value), metadata: previous.Metadata()}
		change.Previous = &copyPrevious
	}
	for _, watcher := range watchers {
		func() { defer func() { _ = recover() }(); _ = watcher(ctx, change) }()
	}
}

func changedKeys[T any](previous *Snapshot[T], current Snapshot[T]) []string {
	if previous == nil {
		return flattenKeys(structMap(current.value))
	}
	before, after := flatten(structMap(previous.value)), flatten(structMap(current.value))
	set := make(map[string]struct{})
	for key, value := range before {
		if !reflect.DeepEqual(value, after[key]) {
			set[key] = struct{}{}
		}
	}
	for key, value := range after {
		if !reflect.DeepEqual(value, before[key]) {
			set[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func checksum(values map[string]any) (string, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("config: checksum: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sendError(ctx context.Context, target chan<- error, err error) {
	select {
	case target <- err:
	case <-ctx.Done():
	}
}
