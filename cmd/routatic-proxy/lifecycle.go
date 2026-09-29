package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/routatic/proxy/internal/gui"
	"github.com/routatic/proxy/internal/server"
)

// Both listeners stop accepting work before either drain is awaited. The proxy
// owns the database, but the dashboard also uses it until its handlers finish.
func shutdownServers(ctx context.Context, proxy *server.Server, dashboard *gui.Server) error {
	guiDone := make(chan error, 1)
	go func() {
		if dashboard != nil {
			guiDone <- dashboard.Shutdown(ctx)
		} else {
			guiDone <- nil
		}
	}()
	err := errors.Join(proxy.Shutdown(ctx), <-guiDone)
	if err != nil {
		return fmt.Errorf("HTTP shutdown incomplete (shared resources retained): %w", err)
	}
	return proxy.Close()
}
