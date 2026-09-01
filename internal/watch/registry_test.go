package watch

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRegistryNormalizesKeysAndCancels(t *testing.T) {
	registry := NewRegistry()
	var calls int
	cancel := registry.Subscribe([]string{" Feature.Enabled ", "feature.enabled", ""}, func(key, value string) {
		calls++
		if key != "feature.enabled" || value != "true" {
			t.Fatalf("callback received (%q, %q)", key, value)
		}
	})

	registry.Notify(" FEATURE.ENABLED ", "true")
	if calls != 1 {
		t.Fatalf("callback count = %d, want 1", calls)
	}
	if keys := registry.Keys(); len(keys) != 1 || keys[0] != "feature.enabled" {
		t.Fatalf("keys = %#v", keys)
	}

	cancel()
	cancel()
	registry.Notify("feature.enabled", "true")
	if calls != 1 {
		t.Fatalf("callback count after cancel = %d, want 1", calls)
	}
	if keys := registry.Keys(); len(keys) != 0 {
		t.Fatalf("keys after cancel = %#v", keys)
	}
}

func TestRegistryCallbackCanCancelItself(t *testing.T) {
	registry := NewRegistry()
	var cancel func()
	cancel = registry.Subscribe([]string{"key"}, func(_, _ string) {
		cancel()
	})

	registry.Notify("key", "value")
	if keys := registry.Keys(); len(keys) != 0 {
		t.Fatalf("keys after callback cancellation = %#v", keys)
	}
}

func TestRegistryNotifyAll(t *testing.T) {
	registry := NewRegistry()
	values := make(map[string]string)
	registry.Subscribe([]string{"second", "first"}, func(key, value string) {
		values[key] = value
	})

	registry.NotifyAll(func(key string) string { return key + "-value" })
	if values["first"] != "first-value" || values["second"] != "second-value" {
		t.Fatalf("notified values = %#v", values)
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	registry := NewRegistry()
	var calls atomic.Int64
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			cancel := registry.Subscribe([]string{"key"}, func(_, _ string) {
				calls.Add(1)
			})
			registry.Notify("key", "value")
			_ = registry.Keys()
			cancel()
		}()
	}
	group.Wait()

	if calls.Load() == 0 {
		t.Fatal("callbacks were not invoked")
	}
}
