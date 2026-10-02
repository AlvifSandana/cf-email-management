# Email Management Service — System Design

**Version:** 0.1.0  
**Status:** Implemented / Production Ready  
**Date:** 2026-10-02

---

## 1. Architecture

```text
               ┌────────────────────────────────────────────────────────┐
               │              Web Dashboard / CLI / API Clients         │
               └───────────────────────────┬────────────────────────────┘
                                           │ HTTPS
                                           ▼
               ┌────────────────────────────────────────────────────────┐
               │                 Traefik Reverse Proxy                  │
               │         Auto-TLS / Security Headers / Rate Limit       │
               └───────────────────────────┬────────────────────────────┘
                                           │
                                           ▼
┌──────────────────────────────────────────────────────────────────────────────────┐
│                             EMS Modular Monolith                                 │
│                                                                                  │
│   ┌──────────────────────────────────────────────────────────────────────────┐   │
│   │                 Authentication & Access Control Layer                    │   │
│   │   • Static X-API-Key (Admin fallback)   • OIDC JWT Bearer Validation     │   │
│   │   • Role Hierarchy (Admin > Operator > Viewer) • Permission Middleware   │   │
│   └─────────────────────────────────────┬────────────────────────────────────┘   │
│                                         │                                        │
│                                         ▼                                        │
│   ┌──────────────────────────────────────────────────────────────────────────┐   │
│   │                         Application Services                             │   │
│   │   • Destination Service (Verification Enforcement)                       │   │
│   │   • Routing Service (Explicit Rules & Dedicated Catch-All)               │   │
│   │   • Outbound Email Service (Cloudflare Sending API Client)               │   │
│   │   • Sync Engine (Diff: MATCHED, LOCAL_ONLY, REMOTE_ONLY, CHANGED)        │   │
│   │   • Background Reconciliation Worker (Ticker loop & Drift Callback)      │   │
│   │   • Notification Alerting Service (Slack, Discord, Telegram, Generic)    │   │
│   │   • Audit Service (Automatic Secret Redaction)                           │   │
│   └──────────────────────┬───────────────────────────────────┬───────────────┘   │
│                          │                                   │                   │
│                          ▼                                   ▼                   │
│               ┌─────────────────────┐             ┌─────────────────────┐        │
│               │   Storage Engine    │             │   Provider Layer    │        │
│               │ (PostgreSQL 16/Mem) │             │(EmailProvider Abstr)│        │
│               └──────────┬──────────┘             └──────────┬──────────┘        │
└──────────────────────────┼───────────────────────────────────┼───────────────────┘
                           │                                   │
                           ▼                                   ▼
                   PostgreSQL Database                Remote Providers
                   (AES-256-GCM Encrypted)       ├── Cloudflare API v4 & Workers
                                                 └── AWS SES (Domain & Rules)
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

## 3. Repository Layout

```text
email-management-service/
├── .github/
│   └── workflows/ci.yml         # GitHub Actions CI/CD Pipeline
├── cmd/
│   ├── ems/                     # EMS daemon bootstrapping & graceful shutdown
│   └── ems-cli/                 # Dedicated CLI management tool
├── internal/
│   ├── audit/                   # Audit logging with secret redaction
│   ├── auth/                    # AES-256-GCM encryption & OIDC JWT validation
│   │   └── rbac/                # Role hierarchy (Admin, Operator, Viewer) & permissions
│   ├── config/                  # Environment loader
│   ├── destination/             # Destination address verification & CRUD
│   ├── domain/                  # Entities, value objects & normalized AppErrors
│   ├── email/
│   │   └── sending/             # Cloudflare outbound email sending client
│   ├── httpapi/                 # REST controllers, middleware & embedded dashboard
│   ├── notification/            # Multi-channel webhooks (Slack, Discord, Telegram, Generic)
│   ├── observability/           # Probes (/healthz, /readyz) & Prometheus metrics
│   ├── provider/
│   │   ├── cloudflare/          # Cloudflare API v4 adapter & Worker deployment
│   │   ├── mock/                # In-memory test doubles
│   │   ├── ses/                 # Multi-provider AWS SES adapter
│   │   └── provider.go          # Abstract EmailProvider interface
│   ├── routing/                 # Explicit Rules & Dedicated Catch-all services
│   ├── storage/                 # Repository interfaces with Postgres & Memory
│   └── sync/                    # Sync engine & background reconciliation worker
├── migrations/                  # Auto-applied SQL schema migrations
├── scripts/                     # Automated backup & restore runbooks
├── web/                         # Embedded single-page Web Dashboard
├── api/
│   └── openapi.yaml             # OpenAPI 3.1 specification
├── Dockerfile                   # Multi-stage hardened non-root container
├── compose.yaml                 # Local development Compose
├── compose.prod.yaml            # Hardened production Compose with Traefik
├── Makefile                     # Build & test targets
└── README.md
```

---

### AD-007 — Scheduled Background Reconciliation Loop

A background ticker-driven worker periodically reconciles remote zones against local state to detect external configuration drift without blocking incoming API traffic.

### AD-008 — Multi-Channel Webhook Notifications

Drift events and destination verification confirmations trigger asynchronous notifications formatted specifically for Slack (Block Kit), Discord (Embeds), Telegram (Markdown), or Generic JSON webhooks.

### AD-009 — Decoupled Outbound Email Sending Domain

Outbound sending (`internal/email/sending`) is decoupled from inbound routing logic to isolate concerns and normalize Cloudflare Send API error envelopes into canonical domain errors.

### AD-010 — Multi-User RBAC & OIDC Authentication Layer

Access control implements a 3-tier hierarchy (Admin > Operator > Viewer) enforceable by route middleware, supporting both static admin API keys and OIDC JWT bearer tokens with signature and role claim validation.

### AD-011 — Multi-Provider Extensibility (AWS SES)

The provider layer implements `provider.EmailProvider` for both Cloudflare API v4 and AWS SES (mapping SES domain identities, receipt rule sets, and verified email identities).

### AD-012 — Cloudflare Worker Script Deployment Management

Allows programmatic deployment, inspection, and removal of Cloudflare Workers scripts directly via EMS to support dynamic worker-based email routing actions.

### AD-013 — Embedded Web Dashboard and CLI Management Tool

Provides dual administrative interfaces: an embedded single-page responsive dashboard (`web/index.html`) served directly by the Go binary, and a standalone CLI tool (`cmd/ems-cli`) for terminal workflows.

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

## 16. Cloudflare Email Sending (Implemented)

Outbound sending is implemented as an isolated client module:

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
