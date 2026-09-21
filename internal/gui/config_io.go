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
	data, err := json.Marshal(cfg)
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
				if len(v) > 0 {
					m[key] = []string{keyMask}
				}
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
		http.Error(w, "config not available", http.StatusServiceUnavailable)
		return
	}

	cfg, err := anonymizeConfig(s.atomicCfg.Get())
	if err != nil {
		http.Error(w, "failed to export config", http.StatusInternalServerError)
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
		http.Error(w, "config not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Config json.RawMessage `json:"config"`
		Apply  bool            `json:"apply"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	var patch map[string]json.RawMessage
	if err := json.Unmarshal(req.Config, &patch); err != nil {
		http.Error(w, fmt.Sprintf("invalid config: %v", err), http.StatusBadRequest)
		return
	}
	cfg, err := s.updateProxyConfig(patch, req.Apply)
	if err != nil {
		writeConfigError(w, err)
		return
	}
	redacted, err := anonymizeConfig(cfg)
	if err != nil {
		http.Error(w, "failed to redact config", http.StatusInternalServerError)
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
		return nil, errors.New("config must be a JSON object")
	}
	if err := stripMaskedKeys(patch); err != nil {
		return nil, err
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
		return nil, fmt.Errorf("failed to parse current config: %w", err)
	}
	if merged == nil {
		return nil, errors.New("current config must be a JSON object")
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
		if err := writeConfigFile(configPath, data); err != nil {
			return nil, err
		}
		s.atomicCfg.ApplyLoaded(cfg)
	}
	return cfg, nil
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

func writeConfigError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var pathErr *os.PathError
	var linkErr *os.LinkError
	if errors.As(err, &pathErr) || errors.As(err, &linkErr) {
		status = http.StatusInternalServerError
	}
	http.Error(w, err.Error(), status)
}
