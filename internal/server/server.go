// Package server runs the HTTP server that serves the site.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/gobuffalo/pop/v6"

	"github.com/fbriansyah/go-personal-web/internal/admin"
	"github.com/fbriansyah/go-personal-web/internal/config"
	"github.com/fbriansyah/go-personal-web/internal/content"
	"github.com/fbriansyah/go-personal-web/internal/database"
)

// Run serves until ctx is cancelled, then drains in-flight requests within the
// configured shutdown timeout. It returns only once the server has stopped.
func Run(ctx context.Context, cfg *config.Config, conn *pop.Connection, log *slog.Logger) error {
	srv := &http.Server{
		Handler:      routes(cfg, conn, log),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Listen synchronously so that a failure to bind is reported as the error
	// of `serve` rather than arriving asynchronously after it looks started.
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.Port))
	if err != nil {
		return err
	}
	log.Info("listening", "addr", ln.Addr().String())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Info("shutting down", "timeout", cfg.Server.ShutdownTimeout)
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Server.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		log.Info("stopped")
		return nil
	}
}

func routes(cfg *config.Config, conn *pop.Connection, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Admin is mounted only when it has both of the secrets ADR-0009 keeps in
	// config. Without them there is nothing to authenticate against, and
	// mounting an unprotected /admin would be worse than not having one. A
	// fresh clone therefore serves the public site and says why.
	if cfg.Admin.Enabled() {
		mux.Handle("/admin/", admin.New(content.NewStore(conn), cfg, log))
	} else {
		log.Warn("admin is not mounted: set admin.password_hash and admin.session_secret to enable it")
	}

	// healthz answers for the process alone. Failing it when the database is
	// down would have an orchestrator restart a perfectly healthy binary over
	// a Postgres restart it can do nothing about.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})

	// readyz answers for the process and its database, which is what deciding
	// whether to send traffic here actually depends on.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if err := database.Ping(r.Context(), conn); err != nil {
			// The error names the host and may name the user, so the body says
			// only that the check failed.
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "database unavailable")
			return
		}
		fmt.Fprintln(w, "ok")
	})

	return mux
}
