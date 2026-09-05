// Package app is the single composition root for the gkd process.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"gk/api"
	"gk/db/sqlc"
	"gk/internal/platform/config"
	"gk/internal/platform/httpserver"
	"gk/internal/platform/logging"
	"gk/internal/platform/observability"
	"gk/internal/platform/postgres"
	"gk/internal/platform/version"
	"gk/internal/task"
	"gk/web"
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel, cfg.LogFormat, cfg.Environment)
	slog.SetDefault(logger)

	telemetry, err := observability.Init(ctx, cfg.ServiceName, version.Version, cfg.Environment)
	if err != nil {
		return fmt.Errorf("initialize observability: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(shutdownCtx); err != nil {
			logger.Warn("flush telemetry", "error", err)
		}
	}()

	database, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()

	handler := routes(database, logger, cfg.PublicURL)
	server := httpserver.New(cfg.Addr, handler, logger, cfg.ShutdownTimeout)
	return server.Run(ctx)
}

func routes(database *pgxpool.Pool, logger *slog.Logger, publicURL string) http.Handler {
	taskRepository := task.NewPostgresRepository(sqlc.New(database))
	taskService := task.NewService(taskRepository)
	taskHandler := task.NewHTTPHandler(taskService, logger.With("module", "task"))

	mux := http.NewServeMux()
	mux.Handle("/api/", api.Handler(taskHandler))
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := database.Ping(r.Context()); err != nil {
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "database_unavailable", "database is unavailable")
			return
		}
		httpserver.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.Handle("/", web.Handler())

	return otelhttp.NewHandler(httpserver.Chain(mux,
		httpserver.WithRequestID,
		httpserver.Recover(logger),
		httpserver.AccessLog(logger),
		httpserver.CORS(publicURL),
	), "http.server")
}
