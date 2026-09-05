// Package server runs the HTTP server that serves the site.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/fbriansyah/go-personal-web/internal/config"
)

// Run serves until ctx is cancelled, then drains in-flight requests within the
// configured shutdown timeout. It returns only once the server has stopped.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	srv := &http.Server{
		Handler:      routes(),
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

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})
	return mux
}
