# Email Management Service — Production Operations & Hardening Guide

**Version:** 0.1.0  
**Status:** Production Ready  
**Date:** 2026-10-02

---

## 1. Production Architecture

In production, EMS runs behind **Traefik** as a reverse proxy with automated Let's Encrypt TLS and **PostgreSQL 16** with persistent storage.

```text
       Internet (HTTPS:443)
               │
               ▼
       ┌────────────────┐
       │ Traefik Proxy  │  (Auto TLS, ACME Let's Encrypt, HTTP to HTTPS redirect)
       └───────┬────────┘
               │  Docker internal bridge network (ems-network)
               ▼
       ┌────────────────┐
       │   EMS Service  │  (Go binary, Non-root ems user, Read-only FS, Security headers, Rate Limiting)
       └───────┬────────┘
               │  TCP 5432
               ▼
       ┌────────────────┐
       │   PostgreSQL   │  (pgdata volume, auto-migrated schema, pg_isready healthcheck)
       └────────────────┘
```

---

## 2. Hardening Measures Implemented

| Security Layer | Implementation | Verification |
|---|---|---|
| **Container User** | Non-root `ems:ems` UID 1000 | Configured in `Dockerfile` and `compose.prod.yaml` |
| **Filesystem** | `read_only: true` with temporary `/tmp` tmpfs | Prevents filesystem tampering in container |
| **Linux Capabilities** | `cap_drop: [ALL]`, `no-new-privileges:true` | Minimum privilege execution |
| **Credential Encryption** | AES-256-GCM via `EMS_MASTER_KEY` (32 bytes) | Provider tokens encrypted before DB write, decrypted only in-memory |
| **Audit Redaction** | Sensitive fields (`token`, `secret`, `password`, `key`) stripped | Verified in `internal/audit/audit_test.go` |
| **Security Headers** | HSTS, CSP, X-Frame-Options: DENY, X-Content-Type: nosniff | Enforced via `SecurityHeadersMiddleware` |
| **Rate Limiting** | In-memory sliding window per IP with `Retry-After: 60` | Configured via `EMS_RATE_LIMIT` (default 120 req/min) |
| **Network Isolation** | Internal `ems-network` bridge | Database port 5432 is not exposed to public host |
| **Automated Migrations** | Embedded SQL runs on startup via `migrations.InitSchemaSQL` | Eliminates manual DB migration steps |
| **CI/CD Quality Gate** | GitHub Actions with tests, race detection, linters, Trivy scan | Pre-deployment verification in `.github/workflows/ci.yml` |
| **RBAC & OIDC Auth** | JWT bearer token verification with Admin/Operator/Viewer roles | Multi-user least-privilege access control |
| **Alerting Webhooks** | Multi-channel dispatch (Slack, Discord, Telegram, Generic) | Real-time drift and destination alerts |

---

## 3. Deployment Checklist

### Step 1: Prepare Environment File

Copy the production template:
```bash
cp .env.production.example .env
```

Generate production secrets:

1. **Master Encryption Key (32-byte / 64 hex characters):**
   ```bash
   openssl rand -hex 32
   ```
   Assign to `EMS_MASTER_KEY`. **Do not lose this key.**

2. **Admin API Key:**
   ```bash
   openssl rand -base64 32
   ```
   Assign to `EMS_API_KEY`.

3. **PostgreSQL Password:**
   ```bash
   openssl rand -hex 16
   ```
   Assign to `POSTGRES_PASSWORD`.

4. **Domain and ACME Email:**
   Set `EMS_DOMAIN=ems.yourdomain.com` and `ACME_EMAIL=admin@yourdomain.com`.

### Step 2: Launch Production Stack

```bash
docker compose -f compose.prod.yaml up -d --build
```

### Step 3: Verify Health Probes

```bash
# Liveness probe
curl -I https://ems.yourdomain.com/healthz

# Readiness probe
curl -I https://ems.yourdomain.com/readyz

# Prometheus metrics
curl -s https://ems.yourdomain.com/metrics
```

Expected HTTP status: `200 OK`.

---

## 4. Backup & Disaster Recovery

### Automated Daily Backups
Add a crontab entry on the host system to run the backup script daily at 02:00 AM:

```cron
0 2 * * * cd /opt/email-management-service && ./scripts/backup.sh >> /var/log/ems_backup.log 2>&1
```

The script retains backups for 7 days by default (configurable via `RETENTION_DAYS`).

### Disaster Recovery / Restore Runbook
In case of catastrophic database loss or rollback requirement:

```bash
./scripts/restore.sh ./backups/ems_backup_YYYYMMDD_HHMMSS.sql.gz
```

For Windows environments:
```powershell
.\scripts\restore.ps1 -BackupFile .\backups\ems_backup_YYYYMMDD_HHMMSS.sql
```

---

## 5. Secret Rotation Procedure

If rotating `EMS_API_KEY`:
1. Update `EMS_API_KEY` in `.env`.
2. Reload EMS container: `docker compose -f compose.prod.yaml up -d ems`.

If rotating `EMS_MASTER_KEY`:
1. Decrypt existing credentials using old key.
2. Re-encrypt with new 32-byte key.
3. Update database records and `.env`.
---

## 6. CI/CD Pipeline & Quality Assurance

EMS incorporates an enterprise-grade GitHub Actions CI/CD workflow (`.github/workflows/ci.yml`) triggering on pushes and pull requests to `main`:

1. **Test & Coverage Job**: Runs with Go 1.24, tests all packages with `-race`, verifies module checksums with `go mod verify`, and generates atomic coverage reports.
2. **Code Lint & Format Job**: Enforces canonical `gofmt` compliance, runs `go vet ./...`, and runs `golangci-lint` (including `staticcheck`).
3. **Docker Build Job**: Validates that multi-stage `Dockerfile` compiles cleanly to ensure container reproducibility.
4. **Security Scan Job**: Leverages Aqua Security's Trivy scanner to detect vulnerabilities across repository dependencies and container images.
