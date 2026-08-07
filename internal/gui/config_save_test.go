package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyConfigPatch_PreservesEnvPlaceholders asserts that saving an unrelated
// field from the dashboard does not bake a ${VAR} secret into the config file as
// plaintext, and does not inject defaults the user never wrote.
func TestApplyConfigPatch_PreservesEnvPlaceholders(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_OPENCODE_GO_API_KEY", "sk-secret-from-env")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := `{
  "host": "127.0.0.1",
  "port": 3456,
  "opencode_go": {
    "api_key": "${ROUTATIC_PROXY_OPENCODE_GO_API_KEY}",
    "base_url": "https://example.invalid"
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	// The user edits only the port in the settings form.
	patch := map[string]json.RawMessage{"port": json.RawMessage(`3457`)}

	if err := applyConfigPatch(path, patch); err != nil {
		t.Fatalf("applyConfigPatch: %v", err)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back config: %v", err)
	}
	got := string(saved)

	if strings.Contains(got, "sk-secret-from-env") {
		t.Errorf("env secret was baked into the config file as plaintext:\n%s", got)
	}
	if !strings.Contains(got, "${ROUTATIC_PROXY_OPENCODE_GO_API_KEY}") {
		t.Errorf("the ${VAR} placeholder was lost:\n%s", got)
	}
	if !strings.Contains(got, "3457") {
		t.Errorf("the edited port was not persisted:\n%s", got)
	}
	if !strings.Contains(got, "https://example.invalid") {
		t.Errorf("an untouched sibling field was lost:\n%s", got)
	}
}

// TestApplyConfigPatch_RejectsInvalidPort keeps validation behaviour that the
// handler relied on before the patch logic was extracted.
func TestApplyConfigPatch_RejectsInvalidPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := `{"host":"127.0.0.1","port":3456}` + "\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	patch := map[string]json.RawMessage{"port": json.RawMessage(`70000`)}
	if err := applyConfigPatch(path, patch); err == nil {
		t.Fatal("expected an error for an out-of-range port, got nil")
	}

	saved, _ := os.ReadFile(path)
	if string(saved) != original {
		t.Errorf("a rejected patch must leave the file untouched, got:\n%s", saved)
	}
}
