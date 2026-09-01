package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestYAMLSources(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 9000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := YAMLFile(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	server := document.Values["server"].(map[string]any)
	if server["port"] != 9000 {
		t.Fatalf("unexpected values: %#v", document.Values)
	}
	optional, err := OptionalYAMLFile(filepath.Join(directory, "missing.yaml")).Load(context.Background())
	if err != nil || len(optional.Values) != 0 {
		t.Fatalf("unexpected optional result: %#v, %v", optional, err)
	}
}

func TestEnvironmentRequiresPrefixAndBuildsPaths(t *testing.T) {
	if _, err := Environment("").Load(context.Background()); err == nil {
		t.Fatal("expected prefix error")
	}
	t.Setenv("HEXAS_SERVER_PORT", "9100")
	document, err := Environment("HEXAS_").Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := document.Values["server"].(map[string]any)["port"]; got != "9100" {
		t.Fatalf("unexpected port: %#v", got)
	}
}

func TestParseYAMLRejectsInvalidInput(t *testing.T) {
	if _, err := ParseYAML([]byte("server: [")); err == nil {
		t.Fatal("expected parse error")
	}
}
