package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

// Postgres owns the shared PostgreSQL connection pool.
type Postgres struct {
	pool *pgxpool.Pool
}

func (p *Postgres) Queries() *postgresql.Queries { return postgresql.New(p.pool) }

func (p *Postgres) Begin(ctx context.Context) (pgx.Tx, error) { return p.pool.Begin(ctx) }

// NewPostgres builds and verifies a PostgreSQL pool within a bounded timeout.
func NewPostgres(
	ctx context.Context,
	databaseURL string,
	timezone string,
	connectTimeout time.Duration,
	maxConnections int32,
	minConnections int32,
) (*Postgres, error) {
	poolConfig, err := newPoolConfig(databaseURL, timezone, maxConnections, minConnections)
	if err != nil {
		return nil, err
	}

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

func newPoolConfig(databaseURL, timezone string, maxConnections, minConnections int32) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// Do not wrap the parser error because it may contain the credential-bearing URL.
		return nil, errors.New("invalid PostgreSQL configuration")
	}

	poolConfig.MaxConns = maxConnections
	poolConfig.MinConns = minConnections
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "launlog-api"
	poolConfig.ConnConfig.RuntimeParams["timezone"] = timezone
	return poolConfig, nil
}

// Ping verifies that PostgreSQL accepts a request in the provided context.
func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// VerifyLeastPrivilege rejects runtime accounts that can administer the
// database or create/alter schema objects through the public schema.
func (p *Postgres) VerifyLeastPrivilege(ctx context.Context) error {
	var canTruncateAudit, canTruncatePlatformAudit bool
	var superuser, canCreateRole, canCreateDatabase, canCreateSchema bool
	var canUpdateAudit, canDeleteAudit, canUpdatePlatformAudit, canDeletePlatformAudit, canInsertPlatformAdmin, canUpdatePlatformAdmin, canDeletePlatformAdmin, memberOfOwner, memberOfBootstrap, databaseOwner bool
	err := p.pool.QueryRow(ctx, `SELECT has_table_privilege(current_user, 'public.audit_logs', 'TRUNCATE'), has_table_privilege(current_user, 'public.platform_audit_logs', 'TRUNCATE'), r.rolsuper, r.rolcreaterole, r.rolcreatedb,
		has_schema_privilege(current_user, 'public', 'CREATE'),
		has_table_privilege(current_user, 'public.audit_logs', 'UPDATE'),
		has_table_privilege(current_user, 'public.audit_logs', 'DELETE'),
		has_table_privilege(current_user, 'public.platform_audit_logs', 'UPDATE'),
		has_table_privilege(current_user, 'public.platform_audit_logs', 'DELETE'),
		has_table_privilege(current_user, 'public.platform_admins', 'INSERT'),
		has_table_privilege(current_user, 'public.platform_admins', 'UPDATE'),
		has_table_privilege(current_user, 'public.platform_admins', 'DELETE'),
		pg_has_role(current_user, 'launlog_owner', 'MEMBER'),
		COALESCE(pg_has_role(current_user, to_regrole('launlog_bootstrap'), 'MEMBER'), FALSE),
		pg_has_role(current_user, 'pg_database_owner', 'MEMBER')
		FROM pg_roles r WHERE r.rolname=current_user`).Scan(&canTruncateAudit, &canTruncatePlatformAudit, &superuser, &canCreateRole, &canCreateDatabase, &canCreateSchema, &canUpdateAudit, &canDeleteAudit, &canUpdatePlatformAudit, &canDeletePlatformAudit, &canInsertPlatformAdmin, &canUpdatePlatformAdmin, &canDeletePlatformAdmin, &memberOfOwner, &memberOfBootstrap, &databaseOwner)
	if err != nil {
		return errors.New("verify PostgreSQL runtime privileges")
	}
	if canTruncateAudit || canTruncatePlatformAudit || superuser || canCreateRole || canCreateDatabase || canCreateSchema || canUpdateAudit || canDeleteAudit || canUpdatePlatformAudit || canDeletePlatformAudit || canInsertPlatformAdmin || canUpdatePlatformAdmin || canDeletePlatformAdmin || memberOfOwner || memberOfBootstrap || databaseOwner {
		return errors.New("PostgreSQL runtime role has administrative, schema-creation, or audit-mutation privileges")
	}
	return nil
}

// VerifyPlatformAdminReady gates production startup after restricted bootstrap.
func (p *Postgres) VerifyPlatformAdminReady(ctx context.Context) error {
	var count int
	if err := p.pool.QueryRow(ctx, `SELECT count(*) FROM platform_admins WHERE id=1 AND is_active`).Scan(&count); err != nil || count != 1 {
		return errors.New("exactly one active platform administrator must be bootstrapped before startup")
	}
	return nil
}

// Close releases all PostgreSQL connections.
func (p *Postgres) Close() {
	p.pool.Close()
}
