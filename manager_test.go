package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerPublishesOnlyValidSnapshots(t *testing.T) {
	configDir := writeFixture(t, "port: 1", "", "")
	manager := newTestManager(t, newTestLoader(t, configDir))
	target := &validatedConfig{}
	if err := manager.ReloadInto(target); err != nil {
		t.Fatal(err)
	}
	published := manager.Current()

	writeTestFile(t, filepath.Join(configDir, "overlay.yaml"), "port: 0")
	if err := manager.ReloadInto(&validatedConfig{}); err == nil {
		t.Fatal("invalid configuration was published")
	}
	if manager.Current() != published {
		t.Fatal("failed reload replaced the current snapshot")
	}

	writeTestFile(t, filepath.Join(configDir, "overlay.yaml"), "port: [")
	if err := manager.Reload(); err == nil {
		t.Fatal("invalid YAML was accepted")
	}
	if manager.Current() != published {
		t.Fatal("parse failure replaced the current snapshot")
	}
}

func TestManagersAreIsolated(t *testing.T) {
	if _, err := NewManager(nil); err == nil {
		t.Fatal("NewManager(nil) succeeded")
	}
	first := newTestManager(t, newTestLoader(t, writeFixture(t, "value: first", "", "")))
	second := newTestManager(t, newTestLoader(t, writeFixture(t, "value: second", "", "")))
	if err := first.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := second.Reload(); err != nil {
		t.Fatal(err)
	}
	if first.Current().GetString("value") != "first" || second.Current().GetString("value") != "second" {
		t.Fatal("manager snapshots are not isolated")
	}
}

func TestManagerWatcherCanCancelAndReload(t *testing.T) {
	manager := newTestManager(t, newTestLoader(t, writeFixture(t, "value: stable", "", "")))
	calls := 0
	var cancel func()
	var nestedErr error
	cancel = manager.Subscribe([]string{" Value ", "value"}, func(key, value string) {
		calls++
		if key != "value" || value != "stable" {
			t.Fatalf("watcher received (%q, %q)", key, value)
		}
		cancel()
		nestedErr = manager.Reload()
	})

	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	if nestedErr != nil {
		t.Fatalf("nested Reload() error = %v", nestedErr)
	}
	if calls != 1 {
		t.Fatalf("watcher calls = %d, want 1", calls)
	}
}

func TestManagerMemoryLifecycle(t *testing.T) {
	configDir := writeFixture(t, "value: file", "", "")
	manager := newTestManager(t, newTestLoader(t, configDir))
	if _, err := manager.ReloadMemory(); err == nil {
		t.Fatal("ReloadMemory() succeeded before file load")
	}
	if err := manager.SetMemory("value", "memory", "operations"); err == nil {
		t.Fatal("SetMemory() succeeded before file load")
	}
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetMemory("undeclared", "value", "operations"); err == nil {
		t.Fatal("SetMemory() accepted an undeclared key")
	}
	if err := manager.SetMemory("value", "memory", "operations"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.ReloadMemory()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.GetString("value") != "memory" || snapshot.Source("value") != "operations" {
		t.Fatalf("memory value = %q source = %q", snapshot.GetString("value"), snapshot.Source("value"))
	}

	manager.ClearMemory()
	snapshot, err = manager.ReloadMemory()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.GetString("value") != "file" {
		t.Fatalf("value after ClearMemory = %q", snapshot.GetString("value"))
	}

	if err := manager.SetMemory("value", "temporary", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ReloadMemory(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	snapshot, err = manager.ReloadMemory()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.GetString("value") != "file" {
		t.Fatalf("file reload did not clear memory: %q", snapshot.GetString("value"))
	}
}

func TestManagerReloadReadsUpdatedFile(t *testing.T) {
	configDir := writeFixture(t, "value: first", "", "")
	manager := newTestManager(t, newTestLoader(t, configDir))
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "overlay.yaml"), []byte("value: second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := manager.Current().GetString("value"); !strings.EqualFold(got, "second") {
		t.Fatalf("reloaded value = %q", got)
	}
}

func newTestManager(t *testing.T, loader *Loader) *Manager {
	t.Helper()
	manager, err := NewManager(loader)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
