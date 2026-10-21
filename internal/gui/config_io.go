package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

// sensitiveKeyFragments are matched case-insensitively as substrings of a JSON
// field name. Anything that hits is masked before the config leaves the process,
// on both the settings GET path and the config export download.
var sensitiveKeyFragments = []string{
	"apikey", "api_key", "api-key",
	"token", "secret", "password", "credential",
}

// anonymizeConfig returns a copy of cfg with every secret-looking field replaced
// by keyMask. It works on the marshalled form so any nested provider block is
// covered without per-field code, and it never mutates cfg — the running proxy
// keeps using the real keys to authenticate.
func anonymizeConfig(cfg *config.Config) (*config.Config, error) {
	// Fold the two credential fields into one pool before redacting, so the
	// settings form shows the list the runtime actually rotates through. Without
	// this, a key living in the single-key field is served first and never
	// appears in the list - the form would show a pool whose first row is not
	// the credential that answers the request.
	data, err := json.Marshal(cfg.FoldedForKeyDisplay())
	if err != nil {
		return nil, fmt.Errorf("marshal config for export: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode config for export: %w", err)
	}

	anonymizeMap(raw)

	result := &config.Config{}
	data, err = json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal anonymized config: %w", err)
	}
	if err := json.Unmarshal(data, result); err != nil {
		return nil, fmt.Errorf("decode anonymized config: %w", err)
	}
	return result, nil
}

func anonymizeMap(m map[string]interface{}) {
	for key, value := range m {
		if shouldAnonymize(key) {
			switch v := value.(type) {
			case string:
				if v != "" {
					m[key] = keyMask
				}
			case []interface{}:
				// One hint per key, not one hint for the array. Collapsing a
				// pool of N keys into a single placeholder hid both the size of
				// the pool and which key a row stood for, so editing any one of
				// them meant retyping all of them blind. The hint is a label,
				// never a credential: it leaves only the last four characters,
				// and resolveKeyPatch maps it back to the stored key on save.
				hints := make([]string, 0, len(v))
				for _, item := range v {
					if text, ok := item.(string); ok && text != "" {
						hints = append(hints, maskKeyHint(text))
					}
				}
				if len(hints) == 0 {
					m[key] = []string{}
					continue
				}
				m[key] = hints
			}
		} else if nested, ok := value.(map[string]interface{}); ok {
			anonymizeMap(nested)
		}
	}
}

func shouldAnonymize(key string) bool {
	lowerKey := strings.ToLower(key)
	for _, fragment := range sensitiveKeyFragments {
		if strings.Contains(lowerKey, fragment) {
			return true
		}
	}
	return false
}

func (s *Server) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.atomicCfg == nil {
		writeSettingsError(w, http.StatusServiceUnavailable, "config_unavailable", "", false)
		return
	}

	cfg, err := anonymizeConfig(s.atomicCfg.Get())
	if err != nil {
		writeSettingsError(w, http.StatusInternalServerError, "export_failed", "", false)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=routatic-proxy-config.json")
	w.Header().Set("Cache-Control", "no-store")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(cfg)
}

func (s *Server) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.atomicCfg == nil {
		writeSettingsError(w, http.StatusServiceUnavailable, "config_unavailable", "", false)
		return
	}

	var req struct {
		Config json.RawMessage `json:"config"`
		Apply  bool            `json:"apply"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSettingsError(w, http.StatusBadRequest, "invalid_json", "", false)
		return
	}

	var patch map[string]json.RawMessage
	if err := json.Unmarshal(req.Config, &patch); err != nil {
		writeSettingsError(w, http.StatusBadRequest, "object_required", "", false)
		return
	}
	cfg, err := s.updateProxyConfig(patch, req.Apply)
	if err != nil {
		writeConfigError(w, err)
		return
	}
	redacted, err := anonymizeConfig(cfg)
	if err != nil {
		writeSettingsError(w, http.StatusInternalServerError, "redaction_failed", "", req.Apply)
		return
	}

	resp := map[string]interface{}{
		"valid":  true,
		"config": redacted,
	}
	if req.Apply {
		resp["applied"] = true
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, resp)
}

// updateProxyConfig keeps preview and save on the same validation path. Merge
// raw JSON so environment placeholders and omitted defaults never get written
// back as resolved values. Serializing through publication prevents concurrent
// settings saves and imports from losing each other's changes.
func (s *Server) updateProxyConfig(patch map[string]json.RawMessage, apply bool) (*config.Config, error) {
	if patch == nil {
		return nil, &config.ValidationError{Code: "object_required", Err: errors.New("config must be a JSON object")}
	}
	s.proxyConfigMu.Lock()
	defer s.proxyConfigMu.Unlock()

	// Follow an existing config symlink, as the previous in-place write did.
	configPath, err := filepath.EvalSymlinks(s.atomicCfg.Path())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path: %w", err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read current config: %w", err)
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(raw, &merged); err != nil {
		return nil, &config.ValidationError{Code: "stored_config_invalid", Err: fmt.Errorf("failed to parse current config: %w", err)}
	}
	if merged == nil {
		return nil, &config.ValidationError{Code: "stored_config_invalid", Err: errors.New("current config must be a JSON object")}
	}
	// Redaction placeholders and kept-key references resolve against what is
	// on disk, not against the running config: the running copy may carry
	// credentials injected from the environment, and writing one of those to
	// the file would defeat the whole redaction round trip.
	//
	// The stored blocks are folded first so a row index means the same thing on
	// both sides. The dashboard renders the merged pool, so its row 0 is the
	// single key's value when that field is set; against an unfolded file that
	// index would name a different credential, or be refused as stale.
	//
	// This also migrates the file: a save writes the folded shape, so the file
	// an operator opens afterwards matches the one list the form shows. Whether
	// it actually changed is what decides whether a backup is warranted.
	before := string(raw)
	if err := config.FoldKeyPoolBlocksInPlace(merged); err != nil {
		return nil, err
	}
	if err := stripMaskedKeys(patch, merged); err != nil {
		return nil, err
	}
	for field, value := range patch {
		// Settings sends partial provider/connection objects. Routing maps and
		// arrays are full replacements, so removed entries stay removed.
		if isPartialProviderBlock(field) {
			value, err = mergeConfigSettings(merged[field], value)
			if err != nil {
				return nil, fmt.Errorf("failed to merge %s: %w", field, err)
			}
		}
		merged[field] = value
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize config: %w", err)
	}
	data = append(data, '\n')
	cfg, err := config.LoadJSON(data)
	if err != nil {
		return nil, err
	}
	if apply {
		// Only a save that reshapes the file earns a backup. Every save folds
		// the pools, but once the file is already folded a later save writes the
		// same shape again, and a backup per save would bury the one copy that
		// matters under routine churn.
		if reshapedKeyPools(before, string(data)) {
			if _, err := config.BackupConfigFile(configPath, raw); err != nil {
				return nil, err
			}
		}
		if err := writeConfigFile(configPath, data); err != nil {
			return nil, err
		}
		s.atomicCfg.ApplyLoaded(cfg)
	}
	return cfg, nil
}

// reshapedKeyPools reports whether a save changed the config file's key-pool
// shape - that is, whether it folded a single-key field into its array. It
// compares the decoded objects so formatting is not mistaken for a reshape.
func reshapedKeyPools(before, after string) bool {
	var beforeObj, afterObj map[string]json.RawMessage
	if json.Unmarshal([]byte(before), &beforeObj) != nil || json.Unmarshal([]byte(after), &afterObj) != nil {
		// An unreadable before-image is the case where a backup is most
		// warranted, not least.
		return true
	}
	for _, row := range config.KeyPoolBlockNames() {
		if !sameKeyPoolShape(beforeObj, afterObj, row) {
			return true
		}
	}
	return false
}

// sameKeyPoolShape compares one block's single-key field before and after.
func sameKeyPoolShape(before, after map[string]json.RawMessage, jsonKey string) bool {
	read := func(root map[string]json.RawMessage) string {
		if jsonKey != "" {
			var block map[string]json.RawMessage
			if json.Unmarshal(root[jsonKey], &block) != nil {
				return ""
			}
			root = block
		}
		var single string
		_ = json.Unmarshal(root["api_key"], &single)
		return single
	}
	return read(before) == read(after)
}

// isPartialProviderBlock reports whether a top-level config key holds an object
// the Settings tab patches field by field.
//
// Every platform's block belongs here. A block left out is written wholesale,
// so a patch carrying only the fields the user touched replaces the whole
// object and silently drops the rest of that platform's configuration - the
// platform keeps its markup in the form and loses its settings on save.
//
// The platform entries are derived from the registry rather than listed, so a
// new platform cannot be added without its block being mergeable.
func isPartialProviderBlock(field string) bool {
	switch field {
	case "anthropic_first", "logging", "catalog", "storage":
		return true
	}
	_, known := site.Lookup(field)
	return known
}

func mergeConfigSettings(current, patch json.RawMessage) (json.RawMessage, error) {
	var oldFields, fields map[string]json.RawMessage
	if json.Unmarshal(patch, &fields) != nil || len(fields) == 0 ||
		json.Unmarshal(current, &oldFields) != nil || oldFields == nil {
		return patch, nil
	}
	for field, value := range fields {
		merged, err := mergeConfigSettings(oldFields[field], value)
		if err != nil {
			return nil, err
		}
		oldFields[field] = merged
	}
	return json.Marshal(oldFields)
}

func writeConfigFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".routatic-config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	defer func() { _ = file.Close() }()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync config file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close config file: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("failed to replace config file: %w", err)
	}
	return nil
}
