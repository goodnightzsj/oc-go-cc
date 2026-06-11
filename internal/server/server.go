// Package server manages the HTTP server lifecycle.
package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"oc-go-cc/internal/client"
	"oc-go-cc/internal/config"
	"oc-go-cc/internal/handlers"
	"oc-go-cc/internal/lifecycle"
	"oc-go-cc/internal/metrics"
	"oc-go-cc/internal/router"
	"oc-go-cc/internal/token"
)

const defaultShutdownTimeout = 10 * time.Minute

// Server represents the proxy server.
type Server struct {
	atomic          *config.AtomicConfig
	httpSrv         *http.Server
	logger          *slog.Logger
	levelVar        *slog.LevelVar
	lifecycle       *lifecycle.State
	shutdownTimeout time.Duration
}

// NewServer creates a new proxy server.
func NewServer(atomic *config.AtomicConfig) (*Server, error) {
	cfg := atomic.Get()
	levelVar := new(slog.LevelVar)
	levelVar.Set(parseLogLevel(cfg.Logging.Level))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelVar,
	}))
	slog.SetDefault(logger)

	// Initialize components.
	tokenCounter, err := token.NewCounter()
	if err != nil {
		return nil, fmt.Errorf("failed to create token counter: %w", err)
	}

	// Create metrics
	metrics := metrics.New()
	lifecycleState := lifecycle.NewState()

	openCodeClient := client.NewOpenCodeClient(atomic)
	modelRouter := router.NewModelRouter(atomic)
	fallbackHandler := router.NewFallbackHandler(logger, 3, 30*time.Second)

	// Create handlers.
	messagesHandler := handlers.NewMessagesHandler(
		atomic,
		openCodeClient,
		modelRouter,
		fallbackHandler,
		tokenCounter,
		metrics,
		lifecycleState,
	)
	healthHandler := handlers.NewHealthHandler(tokenCounter, fallbackHandler, metrics, lifecycleState)

	// Setup router.
	mux := http.NewServeMux()

	// API routes.
	mux.HandleFunc("/v1/messages", messagesHandler.HandleMessages)
	mux.HandleFunc("/v1/messages/count_tokens", healthHandler.HandleCountTokens)
	mux.HandleFunc("/health", healthHandler.HandleHealth)
	// OpenAI-compatible model list (some clients use /models without /v1/ prefix)
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		cfg := atomic.Get()
		base := strings.TrimRight(cfg.OpenCodeGo.BaseURL, "/")
		modelsURL := base[:strings.LastIndex(base, "/v1/")+4] + "models"
		req, err := http.NewRequestWithContext(r.Context(), "GET", modelsURL, nil)
		if err != nil {
			http.Error(w, "failed to create request", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "upstream request failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		cfg := atomic.Get()
		base := strings.TrimRight(cfg.OpenCodeGo.BaseURL, "/")
		modelsURL := base[:strings.LastIndex(base, "/v1/")+4] + "models"
		req, err := http.NewRequestWithContext(r.Context(), "GET", modelsURL, nil)
		if err != nil {
			http.Error(w, "failed to create request", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "upstream request failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"service":"oc-go-cc","status":"ok","endpoints":{"/v1/messages":"Anthropic Messages API proxy","/v1/models":"OpenCode Go model list","/v1/messages/count_tokens":"tiktoken counter","/health":"health + metrics"}}`))
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done := lifecycleState.BeginRequest()
		defer done()
		mux.ServeHTTP(w, r)
	})

	// Create HTTP server.
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	httpSrv := &http.Server{
		Addr:        addr,
		Handler:     handler,
		ReadTimeout: 30 * time.Second,
		// SSE responses can run for several minutes; a write deadline would
		// terminate healthy streams mid-response.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	srv := &Server{
		atomic:          atomic,
		httpSrv:         httpSrv,
		logger:          logger,
		levelVar:        levelVar,
		lifecycle:       lifecycleState,
		shutdownTimeout: defaultShutdownTimeout,
	}

	// Register callback to update log level on config reload
	atomic.OnReload(func(newCfg *config.Config) {
		levelVar.Set(parseLogLevel(newCfg.Logging.Level))
		logger.Info("log level updated", "level", newCfg.Logging.Level)
	})

	return srv, nil
}

// Start starts the server with graceful shutdown.
func (s *Server) Start() error {
	cfg := s.atomic.Get()
	s.logger.Info("starting oc-go-cc proxy",
		"host", cfg.Host,
		"port", cfg.Port,
		"base_url", cfg.OpenCodeGo.BaseURL,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- s.httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		s.lifecycle.BeginDrain()
		s.logger.Info("shutting down server...",
			"reason", "shutdown_signal_received",
			"active_requests", s.lifecycle.ActiveRequests(),
			"timeout", s.shutdownTimeout,
		)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		shutdownErr := s.httpSrv.Shutdown(shutdownCtx)
		cancel()
		if shutdownErr != nil {
			s.logger.Error("server shutdown failed",
				"error", shutdownErr,
				"active_requests", s.lifecycle.ActiveRequests(),
			)
			_ = s.httpSrv.Close()
		}

		if err := <-serverErrCh; err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server failed: %w", err)
		}

	case err := <-serverErrCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server failed: %w", err)
		}
	}

	s.logger.Info("server stopped", "active_requests", s.lifecycle.ActiveRequests())
	return nil
}

// WritePID writes the current PID to a file.
func WritePID(path string) error {
	pid := os.Getpid()
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// ReadPID reads the PID from a file.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	return pid, err
}

// parseLogLevel converts a string log level to slog.Level.
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
