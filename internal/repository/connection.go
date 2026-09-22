package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres owns the shared PostgreSQL connection pool.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres builds and verifies a PostgreSQL pool within a bounded timeout.
func NewPostgres(
	ctx context.Context,
	databaseURL string,
	connectTimeout time.Duration,
	maxConnections int32,
	minConnections int32,
) (*Postgres, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// Do not wrap the parser error because it may contain the credential-bearing URL.
		return nil, errors.New("invalid PostgreSQL configuration")
	}
	poolConfig.MaxConns = maxConnections
	poolConfig.MinConns = minConnections
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "launlog-api"

	connectContext, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectContext, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	if err := pool.Ping(connectContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return &Postgres{pool: pool}, nil
}

// Ping verifies that PostgreSQL accepts a request in the provided context.
func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// Close releases all PostgreSQL connections.
func (p *Postgres) Close() {
	p.pool.Close()
}
