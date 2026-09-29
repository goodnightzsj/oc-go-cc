package gui

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestLifecycleParentCancellationDoesNotAbortActiveRequest(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROUTATIC_PROXY_GUI_PORT", strconv.Itoa(ln.Addr().(*net.TCPAddr).Port))
	_ = ln.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	s := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), startProxy: func() error {
		close(entered)
		<-release
		return nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url, err := s.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.srv.Close() })
	done := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Post(url+"api/proxy/start", "application/json", nil)
		if err == nil {
			_ = resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	<-s.limitsDone
	select {
	case err := <-done:
		t.Fatalf("parent cancellation interrupted active request: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for range 2 {
		if err := s.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
