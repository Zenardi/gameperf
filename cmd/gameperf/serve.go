package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/zenardi/gameperf/internal/analyzer"
	"github.com/zenardi/gameperf/internal/metrics"
)

var (
	flagServePort     int
	flagServeInterval int
)

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Expose real-time metrics on a Prometheus /metrics endpoint",
		Long: `serve starts a lightweight HTTP server and continuously collects
system metrics at the configured interval, publishing them in Prometheus
exposition format at /metrics.

Point Prometheus at this endpoint, then visualise with Grafana using the
bundled dashboard at grafana/dashboard.json, or spin up the full stack with:

    docker compose up -d`,
		RunE: runServe,
	}
	cmd.Flags().StringSliceVar(&flagGames, "game", defaultGameNames, "Game process name substrings to watch")
	cmd.Flags().IntVar(&flagServePort, "port", 9100, "Port to expose /metrics on")
	cmd.Flags().IntVar(&flagServeInterval, "interval", 5, "Seconds between metric collection cycles")
	return cmd
}

func runServe(_ *cobra.Command, _ []string) error {
	m := metrics.New()

	addr := fmt.Sprintf(":%d", flagServePort)
	srv := &http.Server{
		Addr:    addr,
		Handler: serveHTTPMux(m),
	}

	fmt.Fprintf(os.Stderr, "🎮  gameperf serve — metrics at http://localhost%s/metrics\n", addr)
	fmt.Fprintf(os.Stderr, "    collecting every %ds, watching: %s\n",
		flagServeInterval, strings.Join(flagGames, ", "))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Kick off the background collection loop before accepting requests.
	go serveCollectLoop(ctx, m)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "\nshutting down…")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	}
}

func serveHTTPMux(m *metrics.Metrics) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body>
<h2>gameperf metrics server</h2>
<a href="/metrics">/metrics</a>
</body></html>`)
	})
	return mux
}

func serveCollectLoop(ctx context.Context, m *metrics.Metrics) {
	// Collect immediately so metrics are available on the first scrape.
	serveCollectOnce(m)

	ticker := time.NewTicker(time.Duration(flagServeInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			serveCollectOnce(m)
		}
	}
}

func serveCollectOnce(m *metrics.Metrics) {
	snap, err := analyzer.Collect(flagGames)
	if err != nil {
		fmt.Fprintf(os.Stderr, "collection error: %v\n", err)
		return
	}
	findings := analyzer.Analyze(snap)
	m.UpdateFromSnapshot(snap, findings)
}
