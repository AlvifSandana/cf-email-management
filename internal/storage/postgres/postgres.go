package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/storage"
	"github.com/bariskode/email-management-service/migrations"
	_ "github.com/lib/pq"
)

// DB wraps *sql.DB and implements storage repositories.
type DB struct {
	db *sql.DB
}

// New opens a connection to PostgreSQL, verifies it with a Ping, and runs schema migrations.
func New(databaseURL string) (*DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	// Auto-apply schema migrations
	if _, err := db.ExecContext(ctx, migrations.InitSchemaSQL); err != nil {
		return nil, fmt.Errorf("failed to execute initial schema migration: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Repositories returns the repository instances backed by Postgres.
func (d *DB) Repositories() *storage.Repositories {
	return &storage.Repositories{
		Accounts:     &accountRepo{d.db},
		Zones:        &zoneRepo{d.db},
		Destinations: &destinationRepo{d.db},
		Rules:        &ruleRepo{d.db},
		CatchAll:     &catchAllRepo{d.db},
		SyncRuns:     &syncRunRepo{d.db},
		Audit:        &auditRepo{d.db},
		Idempotency:  &idempotencyRepo{d.db},
	}
}

// --- Account Repo ---
type accountRepo struct{ db *sql.DB }

func (r *accountRepo) Save(ctx context.Context, acc *domain.CloudflareAccount) error {
	query := `
		INSERT INTO cloudflare_accounts (id, name, cloudflare_account_id, credential_ref, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			cloudflare_account_id = EXCLUDED.cloudflare_account_id,
			credential_ref = EXCLUDED.credential_ref,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at`
	_, err := r.db.ExecContext(ctx, query, acc.ID, acc.Name, acc.CloudflareAccountID, acc.CredentialRef, acc.Status, acc.CreatedAt, acc.UpdatedAt)
	return err
}

func (r *accountRepo) Get(ctx context.Context, id string) (*domain.CloudflareAccount, error) {
	query := `SELECT id, name, cloudflare_account_id, credential_ref, status, created_at, updated_at FROM cloudflare_accounts WHERE id = $1`
	var acc domain.CloudflareAccount
	err := r.db.QueryRowContext(ctx, query, id).Scan(&acc.ID, &acc.Name, &acc.CloudflareAccountID, &acc.CredentialRef, &acc.Status, &acc.CreatedAt, &acc.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("account", id)
		}
		return nil, err
	}
	return &acc, nil
}

func (r *accountRepo) List(ctx context.Context) ([]domain.CloudflareAccount, error) {
	query := `SELECT id, name, cloudflare_account_id, credential_ref, status, created_at, updated_at FROM cloudflare_accounts ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.CloudflareAccount
	for rows.Next() {
		var acc domain.CloudflareAccount
		if err := rows.Scan(&acc.ID, &acc.Name, &acc.CloudflareAccountID, &acc.CredentialRef, &acc.Status, &acc.CreatedAt, &acc.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, acc)
	}
	return list, rows.Err()
}

func (r *accountRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM cloudflare_accounts WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewNotFoundError("account", id)
	}
	return nil
}

// --- Zone Repo ---
type zoneRepo struct{ db *sql.DB }

func (r *zoneRepo) Save(ctx context.Context, z *domain.Zone) error {
	query := `
		INSERT INTO zones (id, provider_account_id, provider_zone_id, name, status, email_routing_enabled, last_synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			email_routing_enabled = EXCLUDED.email_routing_enabled,
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at`
	_, err := r.db.ExecContext(ctx, query, z.ID, z.ProviderAccountID, z.ProviderZoneID, z.Name, z.Status, z.EmailRoutingEnabled, z.LastSyncedAt, z.CreatedAt, z.UpdatedAt)
	return err
}

func (r *zoneRepo) Get(ctx context.Context, id string) (*domain.Zone, error) {
	query := `SELECT id, provider_account_id, provider_zone_id, name, status, email_routing_enabled, last_synced_at, created_at, updated_at FROM zones WHERE id = $1`
	var z domain.Zone
	err := r.db.QueryRowContext(ctx, query, id).Scan(&z.ID, &z.ProviderAccountID, &z.ProviderZoneID, &z.Name, &z.Status, &z.EmailRoutingEnabled, &z.LastSyncedAt, &z.CreatedAt, &z.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("zone", id)
		}
		return nil, err
	}
	return &z, nil
}

func (r *zoneRepo) GetByProviderZoneID(ctx context.Context, providerZoneID string) (*domain.Zone, error) {
	query := `SELECT id, provider_account_id, provider_zone_id, name, status, email_routing_enabled, last_synced_at, created_at, updated_at FROM zones WHERE provider_zone_id = $1`
	var z domain.Zone
	err := r.db.QueryRowContext(ctx, query, providerZoneID).Scan(&z.ID, &z.ProviderAccountID, &z.ProviderZoneID, &z.Name, &z.Status, &z.EmailRoutingEnabled, &z.LastSyncedAt, &z.CreatedAt, &z.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("zone", providerZoneID)
		}
		return nil, err
	}
	return &z, nil
}

func (r *zoneRepo) List(ctx context.Context) ([]domain.Zone, error) {
	query := `SELECT id, provider_account_id, provider_zone_id, name, status, email_routing_enabled, last_synced_at, created_at, updated_at FROM zones ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Zone
	for rows.Next() {
		var z domain.Zone
		if err := rows.Scan(&z.ID, &z.ProviderAccountID, &z.ProviderZoneID, &z.Name, &z.Status, &z.EmailRoutingEnabled, &z.LastSyncedAt, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, z)
	}
	return list, rows.Err()
}

func (r *zoneRepo) ListByAccount(ctx context.Context, accountID string) ([]domain.Zone, error) {
	query := `SELECT id, provider_account_id, provider_zone_id, name, status, email_routing_enabled, last_synced_at, created_at, updated_at FROM zones WHERE provider_account_id = $1 ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Zone
	for rows.Next() {
		var z domain.Zone
		if err := rows.Scan(&z.ID, &z.ProviderAccountID, &z.ProviderZoneID, &z.Name, &z.Status, &z.EmailRoutingEnabled, &z.LastSyncedAt, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, z)
	}
	return list, rows.Err()
}

func (r *zoneRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM zones WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewNotFoundError("zone", id)
	}
	return nil
}

// --- Destination Repo ---
type destinationRepo struct{ db *sql.DB }

func (r *destinationRepo) Save(ctx context.Context, d *domain.DestinationAddress) error {
	query := `
		INSERT INTO destination_addresses (id, provider_account_id, provider_address_id, email, verified_at, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			verified_at = EXCLUDED.verified_at,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at`
	_, err := r.db.ExecContext(ctx, query, d.ID, d.ProviderAccountID, d.ProviderAddressID, d.Email, d.VerifiedAt, d.Status, d.CreatedAt, d.UpdatedAt)
	return err
}

func (r *destinationRepo) Get(ctx context.Context, id string) (*domain.DestinationAddress, error) {
	query := `SELECT id, provider_account_id, provider_address_id, email, verified_at, status, created_at, updated_at FROM destination_addresses WHERE id = $1`
	var d domain.DestinationAddress
	err := r.db.QueryRowContext(ctx, query, id).Scan(&d.ID, &d.ProviderAccountID, &d.ProviderAddressID, &d.Email, &d.VerifiedAt, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("destination", id)
		}
		return nil, err
	}
	return &d, nil
}

func (r *destinationRepo) GetByEmail(ctx context.Context, accountID, email string) (*domain.DestinationAddress, error) {
	query := `SELECT id, provider_account_id, provider_address_id, email, verified_at, status, created_at, updated_at FROM destination_addresses WHERE provider_account_id = $1 AND email = $2`
	var d domain.DestinationAddress
	err := r.db.QueryRowContext(ctx, query, accountID, email).Scan(&d.ID, &d.ProviderAccountID, &d.ProviderAddressID, &d.Email, &d.VerifiedAt, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("destination", email)
		}
		return nil, err
	}
	return &d, nil
}

func (r *destinationRepo) List(ctx context.Context) ([]domain.DestinationAddress, error) {
	query := `SELECT id, provider_account_id, provider_address_id, email, verified_at, status, created_at, updated_at FROM destination_addresses ORDER BY email ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.DestinationAddress
	for rows.Next() {
		var d domain.DestinationAddress
		if err := rows.Scan(&d.ID, &d.ProviderAccountID, &d.ProviderAddressID, &d.Email, &d.VerifiedAt, &d.Status, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func (r *destinationRepo) ListByAccount(ctx context.Context, accountID string) ([]domain.DestinationAddress, error) {
	query := `SELECT id, provider_account_id, provider_address_id, email, verified_at, status, created_at, updated_at FROM destination_addresses WHERE provider_account_id = $1 ORDER BY email ASC`
	rows, err := r.db.QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.DestinationAddress
	for rows.Next() {
		var d domain.DestinationAddress
		if err := rows.Scan(&d.ID, &d.ProviderAccountID, &d.ProviderAddressID, &d.Email, &d.VerifiedAt, &d.Status, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func (r *destinationRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM destination_addresses WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewNotFoundError("destination", id)
	}
	return nil
}

// --- Routing Rule Repo ---
type ruleRepo struct{ db *sql.DB }

func (r *ruleRepo) Save(ctx context.Context, rule *domain.RoutingRule) error {
	query := `
		INSERT INTO routing_rules (id, zone_id, provider_rule_id, name, matcher_type, matcher_field, matcher_value, action_type, destination, enabled, source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			matcher_type = EXCLUDED.matcher_type,
			matcher_field = EXCLUDED.matcher_field,
			matcher_value = EXCLUDED.matcher_value,
			action_type = EXCLUDED.action_type,
			destination = EXCLUDED.destination,
			enabled = EXCLUDED.enabled,
			source = EXCLUDED.source,
			updated_at = EXCLUDED.updated_at`
	_, err := r.db.ExecContext(ctx, query, rule.ID, rule.ZoneID, rule.ProviderRuleID, rule.Name, rule.MatcherType, rule.MatcherField, rule.MatcherValue, rule.ActionType, rule.Destination, rule.Enabled, rule.Source, rule.CreatedAt, rule.UpdatedAt)
	return err
}

func (r *ruleRepo) Get(ctx context.Context, id string) (*domain.RoutingRule, error) {
	query := `SELECT id, zone_id, provider_rule_id, name, matcher_type, matcher_field, matcher_value, action_type, destination, enabled, source, created_at, updated_at FROM routing_rules WHERE id = $1`
	var rule domain.RoutingRule
	err := r.db.QueryRowContext(ctx, query, id).Scan(&rule.ID, &rule.ZoneID, &rule.ProviderRuleID, &rule.Name, &rule.MatcherType, &rule.MatcherField, &rule.MatcherValue, &rule.ActionType, &rule.Destination, &rule.Enabled, &rule.Source, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("rule", id)
		}
		return nil, err
	}
	return &rule, nil
}

func (r *ruleRepo) ListByZone(ctx context.Context, zoneID string) ([]domain.RoutingRule, error) {
	query := `SELECT id, zone_id, provider_rule_id, name, matcher_type, matcher_field, matcher_value, action_type, destination, enabled, source, created_at, updated_at FROM routing_rules WHERE zone_id = $1 ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, query, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.RoutingRule
	for rows.Next() {
		var rule domain.RoutingRule
		if err := rows.Scan(&rule.ID, &rule.ZoneID, &rule.ProviderRuleID, &rule.Name, &rule.MatcherType, &rule.MatcherField, &rule.MatcherValue, &rule.ActionType, &rule.Destination, &rule.Enabled, &rule.Source, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, rule)
	}
	return list, rows.Err()
}

func (r *ruleRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM routing_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewNotFoundError("rule", id)
	}
	return nil
}

func (r *ruleRepo) DeleteByZone(ctx context.Context, zoneID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM routing_rules WHERE zone_id = $1`, zoneID)
	return err
}

// --- Catch All Repo ---
type catchAllRepo struct{ db *sql.DB }

func (r *catchAllRepo) Save(ctx context.Context, rule *domain.CatchAllRule) error {
	query := `
		INSERT INTO catch_all_rules (id, zone_id, provider_rule_id, action_type, destination, enabled, source, last_synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (zone_id) DO UPDATE SET
			provider_rule_id = EXCLUDED.provider_rule_id,
			action_type = EXCLUDED.action_type,
			destination = EXCLUDED.destination,
			enabled = EXCLUDED.enabled,
			source = EXCLUDED.source,
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at`
	_, err := r.db.ExecContext(ctx, query, rule.ID, rule.ZoneID, rule.ProviderRuleID, rule.ActionType, rule.Destination, rule.Enabled, rule.Source, rule.LastSyncedAt, rule.CreatedAt, rule.UpdatedAt)
	return err
}

func (r *catchAllRepo) GetByZone(ctx context.Context, zoneID string) (*domain.CatchAllRule, error) {
	query := `SELECT id, zone_id, provider_rule_id, action_type, destination, enabled, source, last_synced_at, created_at, updated_at FROM catch_all_rules WHERE zone_id = $1`
	var rule domain.CatchAllRule
	err := r.db.QueryRowContext(ctx, query, zoneID).Scan(&rule.ID, &rule.ZoneID, &rule.ProviderRuleID, &rule.ActionType, &rule.Destination, &rule.Enabled, &rule.Source, &rule.LastSyncedAt, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("catch-all", zoneID)
		}
		return nil, err
	}
	return &rule, nil
}

func (r *catchAllRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM catch_all_rules WHERE id = $1 OR zone_id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewNotFoundError("catch-all", id)
	}
	return nil
}

// --- Sync Run Repo ---
type syncRunRepo struct{ db *sql.DB }

func (r *syncRunRepo) Save(ctx context.Context, run *domain.SyncRun) error {
	query := `
		INSERT INTO sync_runs (id, zone_id, direction, status, started_at, finished_at, changes_count, error_count, summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			finished_at = EXCLUDED.finished_at,
			changes_count = EXCLUDED.changes_count,
			error_count = EXCLUDED.error_count,
			summary = EXCLUDED.summary`
	_, err := r.db.ExecContext(ctx, query, run.ID, run.ZoneID, run.Direction, run.Status, run.StartedAt, run.FinishedAt, run.ChangesCount, run.ErrorCount, run.Summary)
	return err
}

func (r *syncRunRepo) Get(ctx context.Context, id string) (*domain.SyncRun, error) {
	query := `SELECT id, zone_id, direction, status, started_at, finished_at, changes_count, error_count, summary FROM sync_runs WHERE id = $1`
	var run domain.SyncRun
	err := r.db.QueryRowContext(ctx, query, id).Scan(&run.ID, &run.ZoneID, &run.Direction, &run.Status, &run.StartedAt, &run.FinishedAt, &run.ChangesCount, &run.ErrorCount, &run.Summary)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("sync_run", id)
		}
		return nil, err
	}
	return &run, nil
}

func (r *syncRunRepo) ListByZone(ctx context.Context, zoneID string) ([]domain.SyncRun, error) {
	query := `SELECT id, zone_id, direction, status, started_at, finished_at, changes_count, error_count, summary FROM sync_runs WHERE zone_id = $1 ORDER BY started_at DESC`
	rows, err := r.db.QueryContext(ctx, query, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.SyncRun
	for rows.Next() {
		var run domain.SyncRun
		if err := rows.Scan(&run.ID, &run.ZoneID, &run.Direction, &run.Status, &run.StartedAt, &run.FinishedAt, &run.ChangesCount, &run.ErrorCount, &run.Summary); err != nil {
			return nil, err
		}
		list = append(list, run)
	}
	return list, rows.Err()
}

func (r *syncRunRepo) List(ctx context.Context, limit int) ([]domain.SyncRun, error) {
	query := `SELECT id, zone_id, direction, status, started_at, finished_at, changes_count, error_count, summary FROM sync_runs ORDER BY started_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.SyncRun
	for rows.Next() {
		var run domain.SyncRun
		if err := rows.Scan(&run.ID, &run.ZoneID, &run.Direction, &run.Status, &run.StartedAt, &run.FinishedAt, &run.ChangesCount, &run.ErrorCount, &run.Summary); err != nil {
			return nil, err
		}
		list = append(list, run)
	}
	return list, rows.Err()
}

// --- Audit Repo ---
type auditRepo struct{ db *sql.DB }

func (r *auditRepo) Record(ctx context.Context, event *domain.AuditEvent) error {
	query := `
		INSERT INTO audit_events (id, actor_type, actor_id, operation, resource_type, resource_id, request_id, before_json, after_json, provider_response_json, status, error_code, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::jsonb, NULLIF($9, '')::jsonb, NULLIF($10, '')::jsonb, $11, $12, $13)`
	_, err := r.db.ExecContext(ctx, query, event.ID, event.ActorType, event.ActorID, event.Operation, event.ResourceType, event.ResourceID, event.RequestID, event.BeforeJSON, event.AfterJSON, event.ProviderResponseJSON, event.Status, event.ErrorCode, event.CreatedAt)
	return err
}

func (r *auditRepo) List(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	query := `SELECT id, actor_type, actor_id, operation, resource_type, resource_id, request_id, COALESCE(before_json::text, ''), COALESCE(after_json::text, ''), COALESCE(provider_response_json::text, ''), status, COALESCE(error_code, ''), created_at FROM audit_events ORDER BY created_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.AuditEvent
	for rows.Next() {
		var event domain.AuditEvent
		if err := rows.Scan(&event.ID, &event.ActorType, &event.ActorID, &event.Operation, &event.ResourceType, &event.ResourceID, &event.RequestID, &event.BeforeJSON, &event.AfterJSON, &event.ProviderResponseJSON, &event.Status, &event.ErrorCode, &event.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, event)
	}
	return list, rows.Err()
}

// --- Idempotency Repo ---
type idempotencyRepo struct{ db *sql.DB }

func (r *idempotencyRepo) Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	query := `SELECT key, status_code, body, created_at, expires_at FROM idempotency_keys WHERE key = $1 AND expires_at > NOW()`
	var rec domain.IdempotencyRecord
	err := r.db.QueryRowContext(ctx, query, key).Scan(&rec.Key, &rec.StatusCode, &rec.Body, &rec.CreatedAt, &rec.ExpiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewNotFoundError("idempotency_record", key)
		}
		return nil, err
	}
	return &rec, nil
}

func (r *idempotencyRepo) Save(ctx context.Context, record *domain.IdempotencyRecord) error {
	query := `
		INSERT INTO idempotency_keys (key, status_code, body, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (key) DO UPDATE SET
			status_code = EXCLUDED.status_code,
			body = EXCLUDED.body,
			expires_at = EXCLUDED.expires_at`
	_, err := r.db.ExecContext(ctx, query, record.Key, record.StatusCode, record.Body, record.CreatedAt, record.ExpiresAt)
	return err
}
