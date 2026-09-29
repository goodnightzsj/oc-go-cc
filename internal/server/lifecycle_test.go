package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
)

func TestLifecycleShutdownPreservesActiveDatabaseUser(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	inserted := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_, err := db.DB().Exec("INSERT INTO requests (id, model, start_time) VALUES ('tail', 'synthetic', '2026-09-29T00:00:00Z')")
		inserted <- err
		w.WriteHeader(http.StatusOK)
	})
	s := &Server{atomic: config.NewAtomicConfig(&config.Config{Host: "127.0.0.1", Port: port}, ""),
		httpSrv: &http.Server{Addr: addr, Handler: handler},
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)), storage: db}
	t.Cleanup(func() { _ = s.httpSrv.Close() })
	started := make(chan error, 1)
	go func() { started <- s.Start() }()
	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		deadline := time.Now().Add(time.Second)
		for {
			resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port))
			if err == nil {
				_ = resp.Body.Close()
				requestDone <- nil
				return
			}
			if time.Now().After(deadline) {
				requestDone <- err
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown=%v", err)
	}
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("listener not stopped")
	}
	// ListenAndServe returning must not release a database still used by a handler.
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) && db.DB().Ping() == nil {
		time.Sleep(time.Millisecond)
	}
	unblock()
	if err := <-inserted; err != nil {
		t.Fatalf("in-flight accounting lost: %v", err)
	}
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().Ping(); err != nil {
		t.Fatalf("shared database closed before owner cleanup: %v", err)
	}
	for range 2 {
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DB().Ping(); err == nil {
		t.Fatal("database remains open after Close")
	}
}

func TestLifecycleStorageFailureStopsStartup(t *testing.T) {
	cfg := config.NewAtomicConfig(&config.Config{
		Storage: &config.StorageConfig{DatabasePath: t.TempDir()},
	}, "")
	srv, err := NewServer(cfg, nil)
	if srv != nil {
		_ = srv.Close()
	}
	if err == nil {
		t.Fatal("startup silently disabled persistence")
	}
}
