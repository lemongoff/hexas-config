package config

import (
	"fmt"
	"strings"
	"sync"

	"github.com/lemongoff/hexas-config/internal/watch"
)

type memoryValue struct {
	value  any
	source string
}

type Manager struct {
	reloadMu sync.Mutex
	stateMu  sync.RWMutex
	loader   *Loader
	base     *Snapshot
	current  *Snapshot
	memory   map[string]memoryValue
	watchers *watch.Registry
}

func NewManager(loader *Loader) (*Manager, error) {
	if loader == nil {
		return nil, fmt.Errorf("config: loader is required")
	}
	return &Manager{
		loader:   loader,
		memory:   make(map[string]memoryValue),
		watchers: watch.NewRegistry(),
	}, nil
}

func (manager *Manager) Reload() error {
	return manager.reload(nil, false)
}

func (manager *Manager) ReloadInto(target any) error {
	return manager.reload(target, true)
}

func (manager *Manager) reload(target any, decode bool) error {
	if manager == nil {
		return fmt.Errorf("config: manager is nil")
	}
	if manager.loader == nil {
		return fmt.Errorf("config: manager has no loader")
	}

	manager.reloadMu.Lock()
	snapshot, err := manager.loader.Load()
	if err == nil && decode {
		err = decodeAndValidate(snapshot, target)
	}
	if err != nil {
		manager.reloadMu.Unlock()
		return err
	}

	manager.stateMu.Lock()
	manager.base = snapshot
	manager.current = snapshot
	manager.memory = make(map[string]memoryValue)
	manager.stateMu.Unlock()
	manager.reloadMu.Unlock()

	manager.notify(snapshot)
	return nil
}

func (manager *Manager) Current() *Snapshot {
	if manager == nil {
		return nil
	}
	manager.stateMu.RLock()
	defer manager.stateMu.RUnlock()
	return manager.current
}

func (manager *Manager) SetMemory(key string, value any, source string) error {
	if manager == nil {
		return fmt.Errorf("config: manager is nil")
	}
	key = normalizeKey(key)
	if key == "" {
		return fmt.Errorf("config: memory key is required")
	}
	if strings.TrimSpace(source) == "" {
		source = "memory"
	}

	manager.stateMu.Lock()
	if manager.base == nil {
		manager.stateMu.Unlock()
		return fmt.Errorf("config: no file snapshot has been loaded")
	}
	if !manager.base.IsSet(key) {
		manager.stateMu.Unlock()
		return fmt.Errorf("config: memory key %q is not declared by the template", key)
	}
	manager.memory[key] = memoryValue{value: cloneValue(value), source: source}
	manager.stateMu.Unlock()
	return nil
}

func (manager *Manager) ClearMemory() {
	if manager == nil {
		return
	}
	manager.stateMu.Lock()
	manager.memory = make(map[string]memoryValue)
	manager.stateMu.Unlock()
}

func (manager *Manager) ReloadMemory() (*Snapshot, error) {
	if manager == nil {
		return nil, fmt.Errorf("config: manager is nil")
	}

	manager.reloadMu.Lock()
	manager.stateMu.RLock()
	if manager.base == nil {
		manager.stateMu.RUnlock()
		manager.reloadMu.Unlock()
		return nil, fmt.Errorf("config: no file snapshot has been loaded")
	}
	snapshot := manager.base.clone()
	memory := make(map[string]memoryValue, len(manager.memory))
	for key, value := range manager.memory {
		memory[key] = value
	}
	manager.stateMu.RUnlock()

	for key, value := range memory {
		snapshot.setWithSource(key, value.value, value.source)
	}
	manager.stateMu.Lock()
	manager.current = snapshot
	manager.stateMu.Unlock()
	manager.reloadMu.Unlock()

	manager.notify(snapshot)
	return snapshot, nil
}

func (manager *Manager) Subscribe(keys []string, watcher Watcher) func() {
	if manager == nil {
		return func() {}
	}
	return manager.watchers.Subscribe(keys, watcher)
}

func (manager *Manager) notify(snapshot *Snapshot) {
	manager.watchers.NotifyAll(func(key string) string {
		return snapshot.GetString(key)
	})
}
