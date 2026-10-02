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

## 14. Phase 12 — Dashboard

Suggested SvelteKit application.

Dashboard modules:

```text
Overview
Domains
Rules
Catch-All
Destinations
Sync
Audit
Settings
```

---

## 15. Phase 13 — Production Hardening

Checklist:

- [ ] TLS
- [ ] reverse proxy
- [ ] secret encryption
- [ ] rate limiting
- [ ] security headers
- [ ] backup
- [ ] migrations tested
- [ ] disaster recovery test
- [ ] provider timeout policy
- [ ] audit verification
- [ ] dependency scanning
- [ ] container image scanning
- [ ] non-root container
- [ ] read-only filesystem where practical

---

## 16. Testing Strategy

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

## 17. Suggested Milestones

```text
M0 Foundation
M1 Provider adapter
M2 Destinations
M3 Zones
M4 Rules
M5 Catch-All
M6 Sync
M7 Audit/Auth
M8 Production hardening
M9 Dashboard
```

---

## 18. Definition of Done

A feature is done when:

- implementation exists
- tests exist
- error handling exists
- API documentation exists
- audit behavior exists where applicable
- logs/metrics are adequate
- security review is completed
- integration behavior is verified
