# Email Management Service — System Design

**Version:** 0.1.0  
**Status:** Proposed  
**Date:** 2026-10-02

---

## 1. Architecture

```text
                    ┌─────────────────────────┐
                    │       Web / CLI         │
                    └────────────┬────────────┘
                                 │ HTTPS
                                 ▼
                    ┌─────────────────────────┐
                    │       EMS API            │
                    │  Auth / Validation       │
                    └────────────┬────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │                         │
                    ▼                         ▼
             ┌─────────────┐          ┌─────────────┐
             │ Domain/Routing│         │ Sync Engine │
             │ Services      │         │             │
             └──────┬──────┘          └──────┬──────┘
                    │                        │
                    └───────────┬────────────┘
                                ▼
                       ┌─────────────────┐
                       │ Provider Layer  │
                       │ EmailProvider   │
                       └────────┬────────┘
                                │
                                ▼
                       ┌─────────────────┐
                       │ Cloudflare API  │
                       └─────────────────┘

                       ┌─────────────────┐
                       │   PostgreSQL    │
                       └─────────────────┘
```

---

## 2. Architectural Decisions

### AD-001 — Modular Monolith

Start as a modular monolith.

Reason:

- Small operational footprint.
- Easy deployment.
- Simple transactions.
- Easier debugging.
- No need for microservices at MVP scale.

Potential future extraction:

```text
ems-api
ems-sync-worker
ems-provider-cloudflare
ems-notification-worker
```

---

## 3. Suggested Repository Layout

```text
email-management-service/
├── cmd/
│   └── ems/
│       └── main.go
├── internal/
│   ├── auth/
│   ├── config/
│   ├── domain/
│   ├── provider/
│   │   ├── cloudflare/
│   │   └── provider.go
│   ├── routing/
│   ├── destination/
│   ├── sync/
│   ├── audit/
│   ├── storage/
│   ├── httpapi/
│   └── observability/
├── migrations/
├── api/
│   └── openapi.yaml
├── docs/
├── tests/
├── Dockerfile
├── compose.yaml
├── Makefile
├── go.mod
└── README.md
```

---

## 4. Layering

```text
HTTP Handler
    ↓
Application Service
    ↓
Domain Model
    ↓
Repository / Provider Interface
    ↓
PostgreSQL / Cloudflare
```

Handlers must not contain Cloudflare-specific implementation.

---

## 5. Provider Adapter

```text
internal/provider/provider.go
internal/provider/cloudflare/
```

Cloudflare-specific request/response mapping lives only in the adapter.

This makes testing possible without real Cloudflare calls.

---

## 6. State Ownership

### Cloudflare owns

- Actual zone identifiers.
- Actual Email Routing state.
- Actual provider rule identifiers.
- Actual destination verification.
- Actual routing behavior.

### EMS owns

- Operator metadata.
- Local resource IDs.
- Desired application state.
- Audit history.
- Sync history.
- Credential references.
- Configuration.

EMS must never assume local state is automatically equal to provider state.

---

## 7. Catch-All Design

Catch-all is a dedicated resource:

```text
Zone
 ├── Explicit Rules[]
 └── CatchAll
```

Not:

```text
Zone
 └── Rules[]
      └── matcher=all
```

Reason: Cloudflare exposes a dedicated catch-all API and its lifecycle is distinct.

---

## 8. Destination Model

Destinations belong to the provider account:

```text
Cloudflare Account
 ├── Destination A
 ├── Destination B
 └── Destination C

Zone A ─┐
Zone B ─┼── can reuse destinations
Zone C ─┘
```

EMS should therefore keep destinations separate from zones.

---

## 9. Sync Strategy

### Pull

Provider → EMS:

```text
Cloudflare
   ↓
Normalize
   ↓
Compare
   ↓
Local state
```

### Push

EMS → Provider:

```text
Desired state
   ↓
Validate
   ↓
Provider mutation
   ↓
Read-after-write
   ↓
Persist actual state
```

Read-after-write is preferred for critical mutations.

---

## 10. Failure Handling

### Provider timeout

Do not commit local "success" state.

Record:

```text
operation = UPDATE_RULE
status = FAILED
error_code = PROVIDER_TIMEOUT
```

### Provider mutation succeeds but response is lost

Treat as uncertain.

Recovery:

```text
GET provider resource
    ↓
Does desired state exist?
    ├── yes → reconcile as success
    └── no  → retry safely
```

This is why idempotency and read-after-write matter.

---

## 11. Security Architecture

```text
Client
  │
 HTTPS
  ▼
API Authentication
  │
Authorization
  │
Validation
  │
Service
  │
Secret Manager / encrypted DB
  │
Cloudflare API
```

Security requirements:

- TLS.
- Strong operator credentials.
- API rate limiting.
- Secret encryption.
- Audit logging.
- No token logging.
- Request IDs.
- Optional IP allowlisting for admin deployments.

---

## 12. Authentication

MVP can use one of:

1. Static admin API key.
2. OIDC.
3. Session-based dashboard authentication.

Recommended initial implementation:

```text
API key for machine/API access
+
optional OIDC later
```

Never store raw API keys in plaintext.

---

## 13. Web Dashboard

Dashboard is intentionally separated from backend.

Suggested future stack:

- SvelteKit
- TypeScript
- Tailwind CSS
- EMS REST API

Pages:

```text
/dashboard
/domains
/domains/:id
/domains/:id/rules
/domains/:id/catch-all
/destinations
/sync
/audit
/settings
```

---

## 14. UX for Catch-All

Display:

```text
Catch-All
────────────────────────────
Status: ACTIVE

Action:
  Forward

Destination:
  inbox@gmail.com

[Disable] [Change Destination]
```

Explicit warning:

> Catch-all can receive mail addressed to arbitrary local-parts on this domain.

---

## 15. Configuration

Environment variables:

```text
EMS_ENV=production
EMS_HTTP_ADDR=:8080
EMS_DATABASE_URL=postgres://...
EMS_MASTER_KEY=...
EMS_LOG_LEVEL=info
EMS_RATE_LIMIT=100
```

Cloudflare credentials should preferably be stored through the application secret mechanism rather than global environment variables.

---

## 16. Future Cloudflare Email Sending

Outbound sending should be a separate module:

```text
internal/email/
    routing/
    sending/
```

Cloudflare currently provides an Email Sending REST API at:

```text
POST /accounts/{account_id}/email/sending/send
```

This must not be conflated with inbound Email Routing.

---

## 17. Threat Model

Threats:

- Credential theft.
- SSRF through provider URLs.
- Unauthorized rule modification.
- Destination manipulation.
- Audit tampering.
- Replay of mutations.
- Rate-limit abuse.
- Database credential theft.

Mitigations:

- Fixed provider base URL.
- Strong authorization.
- Input validation.
- Idempotency.
- Encryption.
- Immutable/append-only audit strategy where possible.
- Rate limiting.
- Secret redaction.
- Dependency scanning.

---

## 18. Backup

PostgreSQL backup:

```text
daily full backup
+
point-in-time recovery where practical
```

Application configuration must be backed up separately from provider secrets.

---
