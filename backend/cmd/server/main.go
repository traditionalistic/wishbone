// Command server is the Wishbone backend: public SDK API, admin API + SPA, live preview (contract/api.md).
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wishbone/backend/internal/api"
	"wishbone/backend/internal/compile"
	"wishbone/backend/internal/ops"
	"wishbone/backend/internal/preview"
	"wishbone/backend/internal/store"
)

func main() {
	addr := "localhost:8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = "localhost:" + p
	}
	flag.StringVar(&addr, "addr", addr, "listen address (\":8080\" listens on every interface)")
	contractDir := flag.String("contract", "../contract", "contract directory")
	adminDir := flag.String("admin", "../admin/dist", "built admin SPA")
	withPreview := flag.Bool("preview", true, "serve the live-preview endpoints (public and admin)")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, addr, *contractDir, *adminDir, *withPreview); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, addr, contractDir, adminDir string, withPreview bool) error {
	c, err := compile.LoadContract(contractDir)
	if err != nil {
		return err
	}
	handlers := (&ops.Demo{}).Handlers()
	gqlURL := os.Getenv("WISHBONE_GRAPHQL_URL")
	maps.Copy(handlers, (&ops.Raffle{G: &ops.GQL{URL: gqlURL, Login: os.Getenv("WISHBONE_GRAPHQL_LOGIN")}}).Handlers())
	if u, err := url.Parse(gqlURL); gqlURL == "" || err != nil {
		slog.Warn("WISHBONE_GRAPHQL_URL is not set or invalid; raffle.* operations answer UPSTREAM_FAILED")
	} else {
		slog.Info("raffle upstream", "host", u.Host, "fallbackLogin", os.Getenv("WISHBONE_GRAPHQL_LOGIN") != "")
	}
	if err := ops.Check(handlers, c.Ops); err != nil {
		return err
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres:///wishbone?sslmode=disable"
	}
	if err := store.EnsureDatabase(ctx, dbURL); err != nil {
		return err
	}
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.ClearInFlight(ctx); err != nil {
		return err
	}
	go pruneIdempotency(ctx, st)

	password := os.Getenv("WISHBONE_ADMIN_PASSWORD")
	if password == "" {
		password = "wishbone"
		slog.Warn("WISHBONE_ADMIN_PASSWORD is not set; using the default admin password")
	}
	var hub *preview.Hub // nil: preview endpoints answer NOT_FOUND
	if withPreview {
		hub = preview.New()
		go hub.Run(ctx)
	}
	srv, err := api.New(api.Config{Contract: c, ContractDir: contractDir, AdminDir: adminDir, Store: st, Ops: handlers, Hub: hub, Password: password})
	if err != nil {
		return err
	}
	if err := srv.Recompile(ctx); err != nil {
		return err
	}
	if err := srv.Seed(ctx); err != nil {
		return err
	}

	hs := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	stopped := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stopped <- hs.Shutdown(shutdown)
	}()
	slog.Info("listening", "addr", addr, "contract", contractDir, "templates", len(c.Templates), "preview", withPreview)
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// in-flight requests finish before the deferred st.Close()
	err = <-stopped
	slog.Info("stopped", "err", err)
	return err
}

// pruneIdempotency drops call records older than 24 h now and then every hour.
func pruneIdempotency(ctx context.Context, st *store.Store) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if err := st.PruneIdempotency(ctx, 24*time.Hour); err != nil && ctx.Err() == nil {
			slog.Warn("prune idempotency", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
