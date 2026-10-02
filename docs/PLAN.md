# Email Management Service — Development Plan

**Version:** 0.1.0  
**Status:** Ready for implementation planning  
**Date:** 2026-10-02

---

## 1. Delivery Strategy

Build as a modular Go monolith with PostgreSQL and a Cloudflare provider adapter.

Implementation should proceed in vertical slices so each phase produces a usable increment.

---

## 2. Phase 0 — Repository Foundation

### Tasks

- Initialize Go module.
- Add configuration package.
- Add structured logging.
- Add HTTP server.
- Add graceful shutdown.
- Add Dockerfile.
- Add Compose.
- Add Makefile.
- Add migration framework.
- Add CI.

### Exit Criteria

```text
go test ./...
go vet ./...
docker compose up
GET /healthz → 200
```

---

## 3. Phase 1 — Database

Create migrations for:

```text
cloudflare_accounts
zones
destination_addresses
routing_rules
catch_all_rules
sync_runs
audit_events
idempotency_keys
```

Add indexes and constraints.

---

## 4. Phase 2 — Cloudflare Provider

Implement:

- authentication
- zones
- Email Routing settings
- destinations
- explicit rules
- catch-all

Tests:

- request serialization
- response parsing
- error mapping
- retry behavior
- timeout handling

Use mocked HTTP transport rather than real API calls for unit tests.

---

## 5. Phase 3 — Destination Management

Implement:

```text
POST /destinations
GET  /destinations
GET  /destinations/:id
PATCH /destinations/:id
DELETE /destinations/:id
```

Acceptance:

- unverified destination cannot be used for forward rule.
- verified destination can be used.
- provider errors are normalized.

---

## 6. Phase 4 — Zone Management

Implement:

```text
GET /zones
POST /zones/import
GET /zones/:id
GET /zones/:id/routing
PATCH /zones/:id/routing
```

Support Cloudflare zone discovery.

---

## 7. Phase 5 — Explicit Rules

Implement:

```text
GET
POST
GET/:id
PUT/:id
DELETE/:id
```

MVP:

```text
matcher = literal
field = to
action = forward/drop
```

---

## 8. Phase 6 — Catch-All

Implement:

```text
GET /zones/:id/catch-all
PUT /zones/:id/catch-all
```

Support:

```text
forward
drop
```

Prepare interface for:

```text
worker
```

Tests must specifically verify that catch-all uses the dedicated Cloudflare endpoint.

---

## 9. Phase 7 — Sync Engine

Implement:

```text
sync zone
sync account
detect drift
```

Diff output:

```text
MATCHED
LOCAL_ONLY
REMOTE_ONLY
CHANGED
UNKNOWN
```

Add sync history.

---

## 10. Phase 8 — Audit

Record:

- actor
- request ID
- operation
- resource
- previous state
- new state
- provider result
- status
- error

Never record:

- API tokens
- master encryption key
- authentication secrets

---

## 11. Phase 9 — Authentication

MVP:

```text
X-API-Key
```

Later:

```text
OIDC
RBAC
```

Roles:

```text
admin
operator
viewer
```

---

## 12. Phase 10 — Observability

Add:

- request metrics
- provider metrics
- sync metrics
- structured logs
- trace IDs

---

## 13. Phase 11 — OpenAPI

Document all public API endpoints.

Generate clients later if needed.

---

## 14. Phase 12 — Dashboard & Management CLI (Completed)

Implemented as a dual-interface management layer:
1. **Embedded Single-Page Dashboard** (`web/index.html`) embedded directly into Go binary and served at `GET /` and `GET /dashboard`.
2. **Dedicated CLI Utility** (`cmd/ems-cli/main.go`) supporting terminal automation for zones, rules, catch-all, destinations, and sync.

---

## 15. Phase 13 — Production Hardening (Completed)

Checklist:

- [x] TLS (Automated via Traefik v3 & Let's Encrypt)
- [x] reverse proxy (`compose.prod.yaml` with Traefik)
- [x] secret encryption (AES-256-GCM via `EMS_MASTER_KEY`)
- [x] rate limiting (Sliding window per IP with `Retry-After`)
- [x] security headers (HSTS, CSP, X-Frame-Options: DENY, nosniff)
- [x] backup (`scripts/backup.sh` and `scripts/backup.ps1`)
- [x] migrations tested (`migrations.InitSchemaSQL` auto-applied on startup)
- [x] disaster recovery test (`scripts/restore.sh` and `scripts/restore.ps1`)
- [x] provider timeout policy (10s default context timeouts)
- [x] audit verification (sanitizes tokens, keys, passwords)
- [x] dependency scanning (via Trivy action in CI)
- [x] container image scanning (via Trivy action in CI)
- [x] non-root container (`ems:ems` UID 1000)
- [x] read-only filesystem where practical (`read_only: true` with tmpfs `/tmp`)

---

## 16. Phase 14 — Scheduled Background Reconciliation Worker (Completed)

- Background ticker-driven worker in `internal/sync/worker.go`.
- Periodic zone drift detection with graceful shutdown on `ctx.Done()`.
- Isolated zone failure handling and `onDriftFunc` event callbacks.
- Comprehensive unit tests in `internal/sync/worker_test.go`.

---

## 17. Phase 15 — Multi-Channel Webhook Alerting Service (Completed)

- Implemented in `internal/notification/service.go`.
- Formatted payloads for **Slack**, **Discord**, **Telegram**, and **Generic JSON**.
- Drift alerts and destination verification alerts.
- Bounded HTTP client timeouts and full unit tests in `internal/notification/service_test.go`.

---

## 18. Phase 16 — Cloudflare Outbound Email Sending Client (Completed)

- Implemented in `internal/email/sending/sending.go`.
- Integration with Cloudflare REST API `POST /accounts/{account_id}/email/sending/send`.
- DTOs, payload translation, and error mapping to `domain.AppError`.
- Comprehensive unit tests in `internal/email/sending/sending_test.go`.

---

## 19. Phase 17 — Multi-User RBAC & OIDC Authentication (Completed)

- Role-based access control in `internal/auth/rbac/rbac.go` (Admin, Operator, Viewer).
- Granular permission matrix and `RequireRole` / `RequirePermission` middleware (403 Forbidden).
- OIDC JWT token validation and HMAC signature checking in `internal/auth/oidc.go`.
- Combined API Key + OIDC middleware and unit tests in `internal/auth/rbac/rbac_test.go`.

---

## 20. Phase 18 — Cloudflare Worker Script Deployment Management (Completed)

- Implemented in `internal/provider/cloudflare/workers.go`.
- API methods for `ListWorkers`, `UploadWorker`, and `DeleteWorker`.
- Error normalization and unit tests in `internal/provider/cloudflare/workers_test.go`.

---

## 21. Phase 19 — Multi-Provider AWS SES Adapter (Completed)

- Implemented in `internal/provider/ses/ses.go`.
- Full implementation of `provider.EmailProvider` for AWS SES domain identities, receipt rules, and verified email identities.
- Unit tests in `internal/provider/ses/ses_test.go`.

---

## 22. Phase 20 — Production CI/CD Pipeline (Completed)

- GitHub Actions workflow in `.github/workflows/ci.yml`.
- Triggers on `push` and `pull_request` targeting `main`.
- Jobs: `test` (race + coverage), `lint` (gofmt, vet, golangci-lint), `docker-build`, and `security-scan` (Trivy).

---

## 23. Testing Strategy

### Unit

- domain validation
- normalization
- diff engine
- service logic
- provider mapping

### Integration

- PostgreSQL
- HTTP provider mocks
- transaction behavior

### End-to-End

Use a dedicated Cloudflare test account/zone.

Never run destructive tests against production domains.

---

## 24. Milestone Status

```text
M0 Foundation                  100% Completed
M1 Provider adapter (CF & SES) 100% Completed
M2 Destinations                100% Completed
M3 Zones                       100% Completed
M4 Rules                       100% Completed
M5 Catch-All                   100% Completed
M6 Sync & Reconciliation       100% Completed
M7 Audit, Auth (RBAC & OIDC)   100% Completed
M8 Production hardening        100% Completed
M9 Web Dashboard & CLI         100% Completed
M10 Advanced Services & CI/CD  100% Completed
```

---

## 25. Definition of Done

A feature is done when:

- implementation exists
- tests exist
- error handling exists
- API documentation exists
- audit behavior exists where applicable
- logs/metrics are adequate
- security review is completed
- integration behavior is verified

