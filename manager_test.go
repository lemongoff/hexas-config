package config

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type testConfig struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Feature struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"feature"`
	Credentials map[string]string `yaml:"credentials"`
}

func defaultTestConfig() testConfig {
	var value testConfig
	value.Server.Port = 8080
	value.Credentials = map[string]string{"token": "default-secret"}
	return value
}

func (c testConfig) Validate() error {
	if c.Server.Port <= 0 {
		return errors.New("port must be positive")
	}
	return nil
}

func TestManagerLoadsOrderedSourcesAndDetachesSnapshots(t *testing.T) {
	manager, err := NewManager(defaultTestConfig(),
		YAMLBytes("base", []byte("server:\n  port: 9000\nfeature:\n  enabled: false\n")),
		Values("override", map[string]any{"feature": map[string]any{"enabled": true}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, ok := manager.Current()
	if !ok {
		t.Fatal("missing current snapshot")
	}
	value := snapshot.Value()
	if value.Server.Port != 9000 || !value.Feature.Enabled {
		t.Fatalf("unexpected value: %+v", value)
	}
	value.Credentials["token"] = "changed"
	again, _ := manager.Current()
	if got := again.Value().Credentials["token"]; got != "default-secret" {
		t.Fatalf("snapshot leaked mutation: %q", got)
	}
	if snapshot.Metadata().Checksum == "" {
		t.Fatal("missing checksum")
	}
}

func TestManagerRejectsUnknownAndInvalidValuesWithoutPublishing(t *testing.T) {
	source := &mutableSource{name: "runtime", values: map[string]any{"server": map[string]any{"port": 9000}}}
	manager, err := NewManager(defaultTestConfig(), source)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	source.set(map[string]any{"server": map[string]any{"port": 0}})
	if err := manager.Load(context.Background()); err == nil {
		t.Fatal("expected validation failure")
	}
	current, _ := manager.Current()
	if current.Value().Server.Port != 9000 {
		t.Fatal("invalid value was published")
	}
	source.set(map[string]any{"unknown": true})
	if err := manager.Load(context.Background()); err == nil {
		t.Fatal("expected strict decode failure")
	}
}

func TestMemoryOverridesAreValidated(t *testing.T) {
	manager, err := NewManager(defaultTestConfig(), Values("base", map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetMemory("server.port", 0); err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background()); err == nil {
		t.Fatal("expected invalid memory override")
	}
	current, _ := manager.Current()
	if current.Value().Server.Port != 8080 {
		t.Fatal("invalid memory value was published")
	}
	manager.ClearMemory()
	if err := manager.SetMemory("server.port", 9100); err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, _ = manager.Current()
	if current.Value().Server.Port != 9100 {
		t.Fatal("valid memory value was not published")
	}
}

func TestWatcherRunsOutsideLocksAndCanCancel(t *testing.T) {
	manager, err := NewManager(defaultTestConfig(), Values("base", map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	var cancel func()
	cancel = manager.Subscribe(func(_ context.Context, change Change[testConfig]) error {
		called++
		if len(change.Changed) == 0 {
			t.Error("missing changed keys")
		}
		cancel()
		return nil
	})
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("watcher called %d times", called)
	}
}

func TestDumpRedactsSecrets(t *testing.T) {
	manager, err := NewManager(defaultTestConfig(), Values("base", map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := manager.Current()
	var output bytes.Buffer
	if err := snapshot.DumpTo(&output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "default-secret") {
		t.Fatal("secret leaked")
	}
	if !strings.Contains(output.String(), "[REDACTED]") {
		t.Fatal("missing redaction marker")
	}
}

type mutableSource struct {
	name   string
	mu     sync.RWMutex
	values map[string]any
}

func (s *mutableSource) Name() string { return s.name }
func (s *mutableSource) Load(context.Context) (Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Document{Values: cloneMap(s.values), Revision: Revision(time.Now().String())}, nil
}
func (s *mutableSource) set(values map[string]any) { s.mu.Lock(); s.values = values; s.mu.Unlock() }
