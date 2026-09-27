// Command server is the Wishbone backend: public SDK API, admin API + SPA, live preview (contract/api.md).
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
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
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	flag.StringVar(&addr, "addr", addr, "listen address")
	contractDir := flag.String("contract", "../contract", "contract directory")
	adminDir := flag.String("admin", "../admin/dist", "built admin SPA")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, addr, *contractDir, *adminDir); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, addr, contractDir, adminDir string) error {
	c, err := compile.LoadContract(contractDir)
	if err != nil {
		return err
	}
	handlers := (&ops.Demo{}).Handlers()
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

	password := os.Getenv("WISHBONE_ADMIN_PASSWORD")
	if password == "" {
		password = "wishbone"
		slog.Warn("WISHBONE_ADMIN_PASSWORD is not set; using the default admin password")
	}
	hub := preview.New()
	go hub.Run(ctx)
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

	hs := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(shutdown)
	}()
	slog.Info("listening", "addr", addr, "contract", contractDir, "templates", len(c.Templates))
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
