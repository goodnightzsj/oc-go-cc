package gui

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

// TestAnonymizeConfig_MasksEveryKeyField asserts no raw key survives the
// settings GET path, and that non-secret fields are left untouched. This is the
// single masker behind both /api/proxy/config and /api/config/export.
func TestAnonymizeConfig_MasksEveryKeyField(t *testing.T) {
	cfg := &config.Config{
		APIKey:  "sk-global-real",
		APIKeys: []string{"sk-a", "sk-b"},
		Host:    "127.0.0.1",
		Port:    3456,
	}
	cfg.OpenCodeGo.APIKey = "sk-go-real"
	cfg.OpenCodeGo.APIKeys = []string{"sk-go-1"}
	cfg.OpenCodeZen.APIKey = "sk-zen-real"
	cfg.AWSBedrock.APIKey = "sk-bedrock-real"
	cfg.OpenRouter.APIKey = "sk-router-real"

	got, err := anonymizeConfig(cfg)
	if err != nil {
		t.Fatalf("anonymizeConfig: %v", err)
	}

	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal redacted config: %v", err)
	}
	for _, secret := range []string{
		"sk-global-real", "sk-a", "sk-b", "sk-go-real", "sk-go-1",
		"sk-zen-real", "sk-bedrock-real", "sk-router-real",
	} {
		if bytesContains(blob, secret) {
			t.Errorf("redacted config still contains secret %q\npayload: %s", secret, blob)
		}
	}

	if got.Host != "127.0.0.1" || got.Port != 3456 {
		t.Errorf("non-secret fields altered: host=%q port=%d", got.Host, got.Port)
	}

	// The original must not be mutated: the proxy keeps using it to auth.
	if cfg.APIKey != "sk-global-real" || cfg.OpenCodeGo.APIKey != "sk-go-real" {
		t.Errorf("anonymizeConfig mutated the source config: global=%q go=%q",
			cfg.APIKey, cfg.OpenCodeGo.APIKey)
	}
	if len(cfg.APIKeys) != 2 || cfg.APIKeys[0] != "sk-a" {
		t.Errorf("anonymizeConfig mutated the source APIKeys slice: %v", cfg.APIKeys)
	}
}

// TestStripMaskedKeys_DropsMaskedFields asserts a replayed masked GET response
// cannot overwrite the real key on disk, at any nesting depth.
func TestStripMaskedKeys_DropsMaskedFields(t *testing.T) {
	raw := `{
		"api_key": "` + keyMask + `",
		"port": 3456,
		"api_keys": ["` + keyMask + `", "` + keyMask + `"],
		"opencode_go": {"api_key": "` + keyMask + `", "timeout_ms": 5000},
		"opencode_zen": {"api_key": "sk-user-typed-a-new-key"}
	}`

	var patch map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}

	stripMaskedKeys(patch)

	if _, ok := patch["api_key"]; ok {
		t.Error("top-level masked api_key survived the strip")
	}
	if _, ok := patch["api_keys"]; ok {
		t.Error("all-masked api_keys array survived the strip")
	}
	if _, ok := patch["port"]; !ok {
		t.Error("non-secret field port was dropped")
	}

	goBlob, ok := patch["opencode_go"]
	if !ok {
		t.Fatal("opencode_go was dropped entirely")
	}
	if bytesContains(goBlob, keyMask) {
		t.Errorf("nested masked api_key survived: %s", goBlob)
	}
	if !bytesContains(goBlob, "timeout_ms") {
		t.Errorf("nested non-secret field was dropped: %s", goBlob)
	}

	zenBlob := patch["opencode_zen"]
	if !bytesContains(zenBlob, "sk-user-typed-a-new-key") {
		t.Errorf("a genuinely edited key must be preserved, got: %s", zenBlob)
	}
}

func bytesContains(b []byte, sub string) bool {
	return bytes.Contains(b, []byte(sub))
}
