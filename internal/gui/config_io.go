package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/routatic/proxy/internal/config"
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
				m[key] = []string{keyMask}
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

	var cfg config.Config
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		http.Error(w, fmt.Sprintf("invalid config: %v", err), http.StatusBadRequest)
		return
	}

	if cfg.Host == "" {
		http.Error(w, "host is required", http.StatusBadRequest)
		return
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		http.Error(w, "port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	resp := map[string]interface{}{
		"valid":  true,
		"config": cfg,
	}

	if req.Apply {
		configPath := s.atomicCfg.Path()
		var patch map[string]json.RawMessage
		if err := json.Unmarshal(req.Config, &patch); err != nil {
			http.Error(w, fmt.Sprintf("invalid config: %v", err), http.StatusBadRequest)
			return
		}
		stripMaskedKeys(patch)
		if err := applyConfigPatch(configPath, patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.atomicCfg.Reload(); err != nil {
			http.Error(w, fmt.Sprintf("failed to reload config: %v", err), http.StatusInternalServerError)
			return
		}

		resp["applied"] = true
	}

	writeJSON(w, resp)
}
