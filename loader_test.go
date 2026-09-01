package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderPrecedenceAndRedactedDump(t *testing.T) {
	configDir := writeFixture(t, `
server:
  port: 80
  name: template
  token: ${APP_TOKEN}
payment:
  signing_material: local-test-value
`, `
server:
  port: 81
  name: overlay
`, "APP_TOKEN: file-secret\n")
	t.Setenv("APP_TOKEN", "environment-secret")
	t.Setenv("SERVER_PORT", "82")

	loader, err := NewLoader(
		DirectoryLayout(configDir),
		WithOverrides("runtime", map[string]any{"server.port": 83}),
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.GetInt("server.port"); got != 83 {
		t.Fatalf("server.port = %d, want 83", got)
	}
	if got := snapshot.GetString("server.name"); got != "overlay" {
		t.Fatalf("server.name = %q, want overlay", got)
	}
	if got := snapshot.GetString("server.token"); got != "environment-secret" {
		t.Fatalf("server.token = %q, want environment value", got)
	}
	if source := snapshot.Source("server.port"); source != "runtime" {
		t.Fatalf("server.port source = %q, want runtime", source)
	}

	var output bytes.Buffer
	if err := snapshot.DumpTo(&output, WithSensitiveKeys("payment.signing_material")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "environment-secret") || strings.Contains(output.String(), "local-test-value") {
		t.Fatalf("DumpTo() leaked a sensitive value:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "server.token=[REDACTED]") || !strings.Contains(output.String(), "payment.signing_material=[REDACTED]") {
		t.Fatalf("DumpTo() did not redact sensitive keys:\n%s", output.String())
	}
}

func TestLoaderResolvesEmbeddedPlaceholders(t *testing.T) {
	configDir := writeFixture(t, "endpoint: https://${HOST}:${PORT}/v1\nport: ${PORT}", "", "HOST: api.example\nPORT: 8443")
	loader := newTestLoader(t, configDir)
	snapshot, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.GetString("endpoint"); got != "https://api.example:8443/v1" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := snapshot.GetInt("port"); got != 8443 {
		t.Fatalf("port = %d, want typed placeholder value", got)
	}
}

func TestLoaderRejectsMissingTemplate(t *testing.T) {
	loader, err := NewLoader(Layout{TemplateFile: filepath.Join(t.TempDir(), "missing.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoaderRejectsPlaceholderCycle(t *testing.T) {
	configDir := writeFixture(t, "value: ${A}", "", "A: ${B}\nB: ${A}")
	_, err := newTestLoader(t, configDir).Load()
	if err == nil || !strings.Contains(err.Error(), "placeholder cycle") {
		t.Fatalf("Load() error = %v, want placeholder cycle", err)
	}
}

func TestLoaderRejectsUndefinedPlaceholder(t *testing.T) {
	const key = "GO_CONFIG_TEST_UNDEFINED"
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	configDir := writeFixture(t, "value: ${"+key+"}", "", "")
	_, err := newTestLoader(t, configDir).Load()
	if err == nil || !strings.Contains(err.Error(), "${"+key+"}") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestSnapshotCollectionsAndMetadataAreDetached(t *testing.T) {
	configDir := writeFixture(t, "items:\n  - one\n  - two\nobject:\n  key: value", "", "")
	snapshot, err := newTestLoader(t, configDir).Load()
	if err != nil {
		t.Fatal(err)
	}

	items := snapshot.GetStringSlice("items")
	items[0] = "changed"
	object := snapshot.GetStringMap("object")
	object["key"] = "changed"
	metadata := snapshot.Metadata()
	metadata.PlaceholderSources[0] = "changed"

	if snapshot.GetStringSlice("items")[0] != "one" {
		t.Fatal("string slice mutated snapshot")
	}
	if snapshot.GetStringMap("object")["key"] != "value" {
		t.Fatal("string map mutated snapshot")
	}
	if snapshot.Metadata().PlaceholderSources[0] == "changed" {
		t.Fatal("metadata mutated snapshot")
	}
}

func TestLoaderLoadIntoValidates(t *testing.T) {
	loader := newTestLoader(t, writeFixture(t, "port: 0", "", ""))
	_, err := loader.LoadInto(&validatedConfig{})
	if !errors.Is(err, errInvalidPort) {
		t.Fatalf("LoadInto() error = %v", err)
	}
	if _, err := loader.LoadInto(nil); err == nil || !strings.Contains(err.Error(), "decode target is nil") {
		t.Fatalf("LoadInto(nil) error = %v", err)
	}
}

func TestLayoutAndOverrideValidation(t *testing.T) {
	if _, err := NewLoader(Layout{}); err == nil || !strings.Contains(err.Error(), "template file is required") {
		t.Fatalf("NewLoader() error = %v", err)
	}
	layout := Layout{TemplateFile: "template.yaml"}
	if _, err := NewLoader(layout, WithOverrides("", map[string]any{"key": "value"})); err == nil {
		t.Fatal("empty override source was accepted")
	}
	if _, err := NewLoader(layout, WithOverrides("runtime", map[string]any{"": "value"})); err == nil {
		t.Fatal("empty override key was accepted")
	}
}

var errInvalidPort = errors.New("port must be positive")

type validatedConfig struct {
	Port int `mapstructure:"port"`
}

func (config *validatedConfig) Validate() error {
	if config.Port <= 0 {
		return errInvalidPort
	}
	return nil
}

func newTestLoader(t *testing.T, configDir string) *Loader {
	t.Helper()
	loader, err := NewLoader(DirectoryLayout(configDir))
	if err != nil {
		t.Fatal(err)
	}
	return loader
}

func writeFixture(t *testing.T, template, overlay, replacement string) string {
	t.Helper()
	root := t.TempDir()
	baseDir := filepath.Join(root, "config", "base")
	configDir := filepath.Join(root, "config", "test")
	for _, directory := range []string{baseDir, configDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(baseDir, "template.yaml"), template)
	if overlay != "" {
		writeTestFile(t, filepath.Join(configDir, "overlay.yaml"), overlay)
	}
	if replacement != "" {
		writeTestFile(t, filepath.Join(configDir, "replace.yaml"), replacement)
	}
	return configDir
}

func writeTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
