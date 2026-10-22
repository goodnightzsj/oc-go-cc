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

func TestAnonymizeConfigPreservesEmptyKeyPool(t *testing.T) {
	got, err := anonymizeConfig(&config.Config{APIKeys: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.APIKeys) != 0 {
		t.Fatal("an empty key pool must not be displayed as a configured credential")
	}
}

// A pool must come back as one labelled row per key. Collapsing it to a single
// mask hid how many keys were configured and which row a key occupied, so the
// only available edit was retyping the whole pool blind.
func TestAnonymizeConfigGivesEachPoolEntryItsOwnHint(t *testing.T) {
	cfg := &config.Config{APIKeys: []string{"sk-first-aaaa", "sk-second-bbbb", "sk-third-cccc"}}
	got, err := anonymizeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{maskKeyHint("sk-first-aaaa"), maskKeyHint("sk-second-bbbb"), maskKeyHint("sk-third-cccc")}
	if len(got.APIKeys) != len(want) {
		t.Fatalf("pool rendered %d rows, want %d: %v", len(got.APIKeys), len(want), got.APIKeys)
	}
	for i := range want {
		if got.APIKeys[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got.APIKeys[i], want[i])
		}
	}
	for _, key := range cfg.APIKeys {
		for _, hint := range got.APIKeys {
			if hint != "" && bytesContains([]byte(key), hint) {
				t.Errorf("hint %q reveals a whole key", hint)
			}
		}
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

	if err := stripMaskedKeys(patch, nil); err != nil {
		t.Fatalf("strip masked keys: %v", err)
	}

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

// TestResolveKeyPatchKeepsUneditedRows is the contract the row UI depends on:
// a row the operator did not touch comes back as {keep, hint}, and the stored
// credential is what gets persisted - never the label.
func TestResolveKeyPatchKeepsUneditedRows(t *testing.T) {
	stored := []string{"sk-first-aaaa", "sk-second-bbbb", "sk-third-cccc"}
	storedRaw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	patch := `[
		{"keep":0,"hint":"` + maskKeyHint(stored[0]) + `"},
		{"value":"sk-replacement-zzzz"},
		{"keep":2,"hint":"` + maskKeyHint(stored[2]) + `"}
	]`
	got, err := resolveKeyPatch(json.RawMessage(patch), storedRaw, nil)
	if err != nil {
		t.Fatalf("resolve key patch: %v", err)
	}
	want := []string{stored[0], "sk-replacement-zzzz", stored[2]}
	if len(got) != len(want) {
		t.Fatalf("pool = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("key %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestResolveKeyPatchRefusesAStaleIndex covers the race the hint exists for: the
// pool changed between the GET that drew the rows and the POST that saved them.
// Resolving the index anyway would swap the operator's kept key for a different
// account without saying so.
func TestResolveKeyPatchRefusesAStaleIndex(t *testing.T) {
	stored := []string{"sk-first-aaaa", "sk-second-bbbb"}
	storedRaw, _ := json.Marshal(stored)
	for name, patch := range map[string]string{
		"hint moved":    `[{"keep":0,"hint":"` + maskKeyHint(stored[1]) + `"}]`,
		"index beyond":  `[{"keep":7,"hint":"` + maskKeyHint(stored[0]) + `"}]`,
		"negative":      `[{"keep":-1,"hint":"` + maskKeyHint(stored[0]) + `"}]`,
		"both forms":    `[{"keep":0,"hint":"` + maskKeyHint(stored[0]) + `","value":"sk-new"}]`,
		"neither form":  `[{"hint":"` + maskKeyHint(stored[0]) + `"}]`,
		"label literal": `["` + maskKeyHint(stored[0]) + `","sk-new"]`,
		"wrong shape":   `[42]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveKeyPatch(json.RawMessage(patch), storedRaw, nil); err == nil {
				t.Fatalf("accepted %s, which would write the wrong credential", patch)
			}
		})
	}
}

// A replay of the redacted GET response must still leave the stored pool alone,
// in both the legacy all-mask shape and the per-row label shape.
func TestResolveKeyPatchDropsAReplayedPool(t *testing.T) {
	stored := []string{"sk-first-aaaa", "sk-second-bbbb"}
	storedRaw, _ := json.Marshal(stored)
	for name, patch := range map[string]string{
		"legacy mask": `["` + keyMask + `","` + keyMask + `"]`,
		"row labels":  `["` + maskKeyHint(stored[0]) + `","` + maskKeyHint(stored[1]) + `"]`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := resolveKeyPatch(json.RawMessage(patch), storedRaw, nil)
			if err != nil {
				t.Fatalf("replayed pool must be ignored, not rejected: %v", err)
			}
			if got != nil {
				t.Fatalf("replayed pool produced %v; the stored pool must be left untouched", got)
			}
		})
	}
}

// An explicitly empty list clears the pool; it must not be read as "no change".
func TestResolveKeyPatchAllowsClearingThePool(t *testing.T) {
	storedRaw, _ := json.Marshal([]string{"sk-first-aaaa"})
	got, err := resolveKeyPatch(json.RawMessage(`[]`), storedRaw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("explicit empty list = %v, want an empty slice", got)
	}
}
