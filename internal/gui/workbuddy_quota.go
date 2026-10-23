package gui

import (
	"net/http"
	"strings"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/quota"
)

func (s *Server) handleWorkBuddyQuota(w http.ResponseWriter, r *http.Request, cfg *config.Config, resp quotaResponse) {
	keys := goQuotaKeys(cfg.WorkBuddy.EffectiveAPIKeys(), nil)
	if len(keys) == 0 {
		resp.Status = "not_configured"
		writeJSON(w, resp)
		return
	}
	endpoint, err := quota.WorkBuddyPanelURL(cfg.WorkBuddy.BaseURL)
	if err != nil {
		resp.Status, resp.Error = "error", err.Error()
		writeJSON(w, resp)
		return
	}
	resp.Endpoint = endpoint
	resp.Links = []quotaLink{{Kind: "usage", URL: strings.TrimSuffix(endpoint, "/api") + "/"}}
	if cached := s.cachedQuota(resp.Provider, endpoint, keys, r.URL.Query().Get("refresh") == "1"); cached != nil {
		writeJSON(w, *cached)
		return
	}
	resp.Accounts = fetchQuotaAccounts(r.Context(), resp.Provider, endpoint, keys)
	resp.Status = quotaStatus(resp)
	resp.FetchedAt = time.Now().UTC()
	s.storeQuota(endpoint, keys, resp)
	writeJSON(w, resp)
}
