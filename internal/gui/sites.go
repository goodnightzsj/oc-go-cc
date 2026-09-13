package gui

import (
	"encoding/json"
	"net/http"

	"github.com/routatic/proxy/internal/site"
)

// siteView is one platform as the dashboard needs it: its identity, and whether
// this deployment can actually use it.
type siteView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Order      int    `json:"order"`
	Selectable bool   `json:"selectable"`
}

// handleSites lists the platforms the dashboard offers, in presentation order.
//
// Selectability is decided here rather than in the browser because the
// credential rule - a platform's own keys, plus the global key for the platforms
// allowed to fall back to it - lives in config. Restating it in JavaScript would
// be a second copy of a rule that has already changed once.
//
// Hidden platforms are left out entirely: their stored records still render a
// label and their adapters still work, but nothing in the dashboard can route to
// them.
func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := s.atomicCfg.Get()
	views := make([]siteView, 0, len(site.All()))
	for _, descriptor := range site.Visible() {
		views = append(views, siteView{
			ID:         descriptor.ID,
			Name:       descriptor.DisplayName,
			Order:      descriptor.Order,
			Selectable: len(cfg.ProviderAPIKeys(descriptor.ID)) > 0,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(map[string]any{"sites": views}); err != nil {
		s.logger.Warn("write sites response", "err", err)
	}
}
