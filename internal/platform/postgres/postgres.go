// Package postgres owns connection-pool and schema-migration lifecycle.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"gk/db/migrations"
)

func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.ConnConfig.RuntimeParams["application_name"] = "gkd"
	cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "60s"
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	if err := migrate(cfg); err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := registerMetrics(pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("register database metrics: %w", err)
	}
	return pool, nil
}

func registerMetrics(pool *pgxpool.Pool) error {
	meter := otel.Meter("gk/internal/platform/postgres")
	total, err := meter.Int64ObservableGauge("gk.db.pool.connections.total")
	if err != nil {
		return err
	}
	acquired, err := meter.Int64ObservableGauge("gk.db.pool.connections.acquired")
	if err != nil {
		return err
	}
	idle, err := meter.Int64ObservableGauge("gk.db.pool.connections.idle")
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		stats := pool.Stat()
		observer.ObserveInt64(total, int64(stats.TotalConns()))
		observer.ObserveInt64(acquired, int64(stats.AcquiredConns()))
		observer.ObserveInt64(idle, int64(stats.IdleConns()))
		return nil
	}, total, acquired, idle)
	return err
}

func migrate(cfg *pgxpool.Config) error {
	db := stdlib.OpenDB(*cfg.ConnConfig)
	defer db.Close()
	return migrateDB(db)
}

func migrateDB(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}
