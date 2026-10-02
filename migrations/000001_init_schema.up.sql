-- 000001_init_schema.up.sql
-- Initial schema for Email Management Service

CREATE TABLE IF NOT EXISTS cloudflare_accounts (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    cloudflare_account_id VARCHAR(128) NOT NULL UNIQUE,
    credential_ref TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS zones (
    id VARCHAR(64) PRIMARY KEY,
    provider_account_id VARCHAR(128) NOT NULL REFERENCES cloudflare_accounts(cloudflare_account_id) ON DELETE CASCADE,
    provider_zone_id VARCHAR(128) NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    email_routing_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_zones_provider UNIQUE (provider_account_id, provider_zone_id),
    CONSTRAINT uq_zones_name UNIQUE (provider_account_id, name)
);

CREATE TABLE IF NOT EXISTS destination_addresses (
    id VARCHAR(64) PRIMARY KEY,
    provider_account_id VARCHAR(128) NOT NULL REFERENCES cloudflare_accounts(cloudflare_account_id) ON DELETE CASCADE,
    provider_address_id VARCHAR(128) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    verified_at TIMESTAMPTZ,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_destination_account_email UNIQUE (provider_account_id, email)
);

CREATE TABLE IF NOT EXISTS routing_rules (
    id VARCHAR(64) PRIMARY KEY,
    zone_id VARCHAR(64) NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
    provider_rule_id VARCHAR(128) NOT NULL,
    name VARCHAR(255) NOT NULL,
    matcher_type VARCHAR(32) NOT NULL DEFAULT 'literal',
    matcher_field VARCHAR(32) NOT NULL DEFAULT 'to',
    matcher_value VARCHAR(255) NOT NULL,
    action_type VARCHAR(32) NOT NULL DEFAULT 'forward',
    destination VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    source VARCHAR(32) NOT NULL DEFAULT 'local',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_rules_provider UNIQUE (zone_id, provider_rule_id),
    CONSTRAINT uq_rules_matcher UNIQUE (zone_id, matcher_field, matcher_value)
);

CREATE TABLE IF NOT EXISTS catch_all_rules (
    id VARCHAR(64) PRIMARY KEY,
    zone_id VARCHAR(64) NOT NULL REFERENCES zones(id) ON DELETE CASCADE UNIQUE,
    provider_rule_id VARCHAR(128) NOT NULL,
    action_type VARCHAR(32) NOT NULL DEFAULT 'forward',
    destination VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    source VARCHAR(32) NOT NULL DEFAULT 'local',
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sync_runs (
    id VARCHAR(64) PRIMARY KEY,
    zone_id VARCHAR(64) NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
    direction VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    changes_count INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    summary TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS audit_events (
    id VARCHAR(64) PRIMARY KEY,
    actor_type VARCHAR(32) NOT NULL,
    actor_id VARCHAR(128) NOT NULL,
    operation VARCHAR(64) NOT NULL,
    resource_type VARCHAR(32) NOT NULL,
    resource_id VARCHAR(128) NOT NULL,
    request_id VARCHAR(128) NOT NULL,
    before_json JSONB,
    after_json JSONB,
    provider_response_json JSONB,
    status VARCHAR(32) NOT NULL,
    error_code VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key VARCHAR(255) PRIMARY KEY,
    status_code INTEGER NOT NULL,
    body BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

-- Indexes for frequent queries
CREATE INDEX IF NOT EXISTS idx_zones_account ON zones(provider_account_id);
CREATE INDEX IF NOT EXISTS idx_destinations_account ON destination_addresses(provider_account_id);
CREATE INDEX IF NOT EXISTS idx_rules_zone ON routing_rules(zone_id);
CREATE INDEX IF NOT EXISTS idx_sync_runs_zone ON sync_runs(zone_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_events(resource_type, resource_id);
