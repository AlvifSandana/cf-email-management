-- 000001_init_schema.down.sql
-- Teardown schema for Email Management Service

DROP TABLE IF EXISTS idempotency_keys CASCADE;
DROP TABLE IF EXISTS audit_events CASCADE;
DROP TABLE IF EXISTS sync_runs CASCADE;
DROP TABLE IF EXISTS catch_all_rules CASCADE;
DROP TABLE IF EXISTS routing_rules CASCADE;
DROP TABLE IF EXISTS destination_addresses CASCADE;
DROP TABLE IF EXISTS zones CASCADE;
DROP TABLE IF EXISTS cloudflare_accounts CASCADE;
