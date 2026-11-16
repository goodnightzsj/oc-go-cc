package gui

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/quota"
)

func TestQuotaSuccessfulAccountDoesNotWaitForOptionalDocs(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var usageHits, docsHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docs" {
			docsHits.Add(1)
			<-release
			http.Error(w, "optional docs unavailable", http.StatusServiceUnavailable)
			return
		}
		usageHits.Add(1)
		_, _ = w.Write([]byte(`{"rolling5h":{"usagePercent":25}}`))
	}))
	defer upstream.Close()
	srv := quotaTestServer(t, upstream.URL, "synthetic-review-key")
	srv.modelLimitsURL = []string{upstream.URL + "/docs"}
	gui := httptest.NewServer(http.HandlerFunc(srv.handleQuota))
	defer gui.Close()
	defer unblock()
	client := &http.Client{Timeout: time.Second}
	for range 2 {
		response, err := client.Get(gui.URL + "/api/quota?provider=opencode-go")
		if err != nil {
			t.Fatalf("successful account blocked by optional docs: %v", err)
		}
		var view quotaResponse
		err = json.NewDecoder(response.Body).Decode(&view)
		_ = response.Body.Close()
		if err != nil || view.Status != "available" || len(view.Accounts) != 1 || view.Accounts[0].Report.Rolling5h.UsedPercent != 25 {
			t.Fatalf("account result lost: %+v, %v", view, err)
		}
	}
	if usageHits.Load() != 1 || docsHits.Load() != 0 {
		t.Fatalf("account cache or read-only snapshot changed: usage=%d docs=%d", usageHits.Load(), docsHits.Load())
	}
}

func TestQuotaModelLimitsRefreshLifecycle(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(strconv.FormatBool(stale), func(t *testing.T) {
			entered, canceled := make(chan struct{}), make(chan struct{})
			var docsHits atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/docs" {
					if docsHits.Add(1) == 1 {
						close(entered)
						<-r.Context().Done()
						close(canceled)
					}
					return
				}
				_, _ = io.WriteString(w, `{"rolling5h":{"usagePercent":25}}`)
			}))
			defer upstream.Close()
			srv := quotaTestServer(t, upstream.URL, "synthetic-review-key")
			srv.modelLimitsURL = []string{upstream.URL + "/docs"}
			srv.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			var prior *quota.ModelLimits
			if stale {
				prior = &quota.ModelLimits{FetchedAt: time.Now().Add(-25 * time.Hour), Models: []quota.ModelLimit{{Model: "synthetic", AllowanceUSD: 60}}}
				srv.modelLimits = prior
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("ROUTATIC_PROXY_GUI_PORT", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
			_ = listener.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			address, err := srv.Start(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
				defer stop()
				if err := srv.Shutdown(stopCtx); err != nil {
					t.Error(err)
				}
			}()
			client := &http.Client{Timeout: time.Second}
			for range 4 {
				response, err := client.Get(address + "api/quota?provider=opencode-go&refresh=1")
				if err != nil {
					t.Fatalf("docs refresh blocked account: %v", err)
				}
				var view quotaResponse
				err = json.NewDecoder(response.Body).Decode(&view)
				_ = response.Body.Close()
				if err != nil || view.Status != "available" || (view.ModelLimits != nil) != stale {
					t.Fatalf("snapshot changed while refresh pending: %+v, %v", view, err)
				}
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("quota read did not wake the service refresh loop")
			}
			if docsHits.Load() != 1 {
				t.Fatalf("concurrent reads duplicated optional refreshes: %d", docsHits.Load())
			}
			cancel()
			select {
			case <-srv.limitsDone:
			case <-time.After(time.Second):
				t.Fatal("service cancellation did not stop optional docs refresh")
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("optional docs request did not observe service cancellation")
			}
			if srv.modelLimits != prior {
				t.Fatal("failed refresh replaced the previous snapshot")
			}
		})
	}
}

func TestQuotaClineSlowSupplementKeepsAccount(t *testing.T) {
	var plans atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users/me/plan" {
			plans.Add(1)
			<-r.Context().Done()
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":25}]}}`)
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.ClinePass.BaseURL = upstream.URL + "/api/v1/chat/completions"
	cfg.ClinePass.APIKeys = []string{"synthetic-one", "synthetic-two"}
	srv := &Server{atomicCfg: config.NewAtomicConfig(cfg, "")}
	gui := httptest.NewServer(http.HandlerFunc(srv.handleQuota))
	defer gui.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(gui.URL + "/api/quota?provider=cline-pass")
	if err != nil {
		t.Fatalf("optional plan lookup suppressed accounts: %v", err)
	}
	defer response.Body.Close()
	var view quotaResponse
	if err := json.NewDecoder(response.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Status != "available" || len(view.Accounts) != 2 || plans.Load() != 1 {
		t.Fatalf("supplement budget did not preserve accounts or bound work: %+v plans=%d", view, plans.Load())
	}
	for _, account := range view.Accounts {
		if account.ClinePass == nil || len(account.ClinePass.Windows) != 1 || account.ClinePass.Windows[0].PercentUsed != 25 {
			t.Fatalf("account data lost after supplement timeout: %+v", account)
		}
	}
}

func TestQuotaModelLimitsBackgroundPublishesSnapshot(t *testing.T) {
	release := make(chan struct{})
	var docsHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docs" {
			docsHits.Add(1)
			select {
			case <-release:
				_, _ = io.WriteString(w, docsFixture)
			case <-r.Context().Done():
			}
			return
		}
		_, _ = io.WriteString(w, `{"rolling5h":{"usagePercent":25}}`)
	}))
	defer upstream.Close()
	srv := quotaTestServer(t, upstream.URL, "synthetic-review-key")
	srv.modelLimitsURL = []string{upstream.URL + "/docs"}
	srv.limitsRefresh = make(chan struct{}, 1)
	srv.limitsDone = make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		defer close(srv.limitsDone)
		srv.limitsLoop(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-srv.limitsDone:
		case <-time.After(time.Second):
			t.Error("background refresh did not stop")
		}
	}()
	if cold := getQuota(t, srv, ""); cold.ModelLimits != nil || cold.Status != "available" {
		t.Fatalf("cold query did not return account without docs: %+v", cold)
	}
	close(release)
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for srv.modelLimitsSnapshot() == nil {
		select {
		case <-deadline.C:
			t.Fatal("background refresh did not publish parsed docs")
		case <-tick.C:
		}
	}
	if cached := getQuota(t, srv, ""); !cached.Cached || cached.ModelLimits != nil {
		t.Fatal("background publication rewrote a cached response")
	}
	fresh := getQuota(t, srv, "?refresh=1")
	if fresh.ModelLimits == nil || len(fresh.ModelLimits.Models) != 4 || fresh.Cached || docsHits.Load() != 1 {
		t.Fatalf("fresh account did not attach published snapshot: %+v docs=%d", fresh, docsHits.Load())
	}
}
