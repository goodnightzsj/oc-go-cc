package main

import (
	"context"
	"errors"
	"time"

	"github.com/routatic/proxy/internal/gui"
	"github.com/routatic/proxy/internal/server"
)

// shutdownDrainBudget is how long a SIGTERM waits for in-flight requests. The
// streaming total budget is ten minutes and the unit's TimeoutStopSec is 630s;
// a five-second drain cut every generation longer than five seconds on each
// restart, which is exactly the traffic those budgets exist for.
const shutdownDrainBudget = 11 * time.Minute

// Both listeners stop accepting work before either drain is awaited. The proxy
// owns the database, but the dashboard also uses it until its handlers finish.
//
// Resources are released even when the drain runs out: Close does not wait on
// in-flight queries, and skipping it left the retention batch and the SQLite
// close undone on every timed-out stop.
func shutdownServers(ctx context.Context, proxy *server.Server, dashboard *gui.Server) (drained error, released error) {
	guiDone := make(chan error, 1)
	go func() {
		if dashboard != nil {
			guiDone <- dashboard.Shutdown(ctx)
		} else {
			guiDone <- nil
		}
	}()
	drained = errors.Join(proxy.Shutdown(ctx), <-guiDone)
	return drained, proxy.Close()
}
