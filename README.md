# Email Management Service (EMS)

[![CI](https://github.com/AlvifSandana/cf-email-management/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/AlvifSandana/cf-email-management/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![OpenAPI](https://img.shields.io/badge/OpenAPI-3.1-6BA539?style=flat&logo=openapiinitiative)](./api/openapi.yaml)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker)](./compose.prod.yaml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
[![Status](https://img.shields.io/badge/Status-Production%20Ready-success.svg)](#development-status)

**Email Management Service (EMS)** is a self-hosted control plane engineered for centrally managing **Cloudflare Email Routing** across multiple accounts and domains.


It replaces repetitive manual Cloudflare dashboard workflows with a single, secure, idempotent REST API featuring automated synchronization, real-time drift detection, and tamper-resistant audit trails.

---

## Architecture at a Glance

```text
       Clients / Web / CLI
               │ (HTTPS)
               ▼
       ┌────────────────────────┐
       │ Traefik Reverse Proxy  │ ── Let's Encrypt TLS & HTTP Redirection
       └───────────┬────────────┘
                   │
                   ▼
┌────────────────────────────────────────────────────────┐
│                   EMS Modular Monolith                 │
│                                                        │
│  [Security Headers]  [Rate Limiter]  [X-API-Key Auth]  │
│  [Request-ID Tracer] [Idempotency]   [Structured Logs] │
│                          │                             │
│                          ▼                             │
│  ┌──────────────────────────────────────────────────┐  │
│  │               Application Services               │  │
│  │   • Destination Service   • Routing Service      │  │
│  │   • Sync Engine           • Audit Service        │  │
│  └───────────────────────┬──────────────────────────┘  │
│                          │                             │
│          ┌───────────────┴───────────────┐             │
│          ▼                               ▼             │
│  ┌────────────────┐              ┌────────────────┐    │
│  │ Storage Engine │              │Provider Adapter│    │
│  │ (PostgreSQL 16)│              │  (Cloudflare)  │    │
│  └────────────────┘              └────────────────┘    │
└──────────┬───────────────────────────────┬─────────────┘
           ▼                               ▼
   PostgreSQL Database              Cloudflare API v4
   (AES-256-GCM at rest)            (Email Routing Engine)
```

---

## Key Features

- **Multi-Domain & Multi-Account Orchestration**: Centrally manage all your Cloudflare accounts and zones through a single unified control plane.
- **Dedicated First-Class Catch-All**: Models catch-all rules natively using Cloudflare's dedicated `/catch_all` endpoints, distinct from generic routing rules.
- **Verification-Gated Forwarding**: Prevents misconfigurations by verifying that destination addresses are confirmed before routing rules can forward mail to them.
- **Bi-Directional Sync & Drift Detection**: Detects divergence between local desired state and Cloudflare remote infrastructure (`MATCHED`, `LOCAL_ONLY`, `REMOTE_ONLY`, `CHANGED`).
- **Zero-Trust Secret Security**: Provider API tokens are encrypted at rest using AES-256-GCM with a 32-byte master key. Sensitive tokens and passwords are automatically redacted from audit logs.
- **Idempotent Mutations**: Supports `Idempotency-Key` headers on mutating requests, preventing duplicate rule creation and unintended side-effects.
- **Production-Grade Hardening**: Dockerized with a non-root user (`ems:ems`), read-only root container filesystem, Traefik automated TLS, and sliding-window rate limiting.
- **Observability Native**: Out-of-the-box `/healthz`, `/readyz`, and Prometheus `/metrics` endpoints.

---

## Important Distinction: Routing vs Mailboxes

> [!NOTE]
> **EMS is an email routing control plane, not a mailbox host.**

- **Supported:** Forwarding custom aliases (e.g., `support@bariskode.com` &rarr; `team@gmail.com`), catch-all routing (`*@bariskode.com`), and drop actions.
- **Out of Scope:** IMAP/POP3 servers, webmail interfaces, and local mailbox storage.

---

## Repository Structure

```text
email-management-service/
├── cmd/
│   └── ems/                 # Application entry point (bootstrapping & graceful shutdown)
├── internal/
│   ├── audit/               # Audit service with automatic secret redaction
│   ├── auth/                # AES-256-GCM credential encryption & API-key middleware
│   ├── config/              # Environment configuration loader
│   ├── destination/         # Destination address management & verification checking
│   ├── domain/              # Core domain models, enums, and normalized errors
│   ├── httpapi/             # REST API handlers, routing, idempotency & security middleware
│   ├── observability/       # Health probes (/healthz, /readyz) & Prometheus metrics
│   ├── provider/
│   │   ├── cloudflare/      # Cloudflare API v4 adapter implementation
│   │   ├── mock/            # In-memory mock provider for testing
│   │   └── provider.go      # Abstract EmailProvider interface
│   ├── routing/             # Zone discovery, explicit routing rules & catch-all services
│   ├── storage/             # Repository interfaces with Postgres & Memory implementations
│   └── sync/                # Sync engine, diff calculator & per-zone concurrency locks
├── migrations/              # Embedded SQL migrations (auto-applied on startup)
├── scripts/                 # Automated backup & restore scripts (Linux/macOS & Windows)
├── api/
│   └── openapi.yaml         # OpenAPI 3.1 specification contract
├── docs/                    # Complete product and architecture specifications
├── Dockerfile               # Multi-stage hardened non-root container image
├── compose.yaml             # Development Docker Compose stack
├── compose.prod.yaml        # Production Docker Compose stack with Traefik & TLS
├── Makefile                 # Build, test, and automation targets
└── README.md
```

---

## Quickstart

### Prerequisites

- [Go 1.24+](https://go.dev/) (to run from source)
- [Docker](https://www.docker.com/) & Docker Compose (for containerized execution)
- Cloudflare API Token (with Zone & Email Routing read/write permissions)

### 1. Run Locally with In-Memory Storage

For instant local testing without spinning up a database:

```bash
# Clone the repository
git clone https://github.com/bariskode/email-management-service.git
cd email-management-service

# Run server (defaults to development mode with in-memory storage)
go run ./cmd/ems
```

Verify the service is running:

```bash
curl http://localhost:8080/healthz
# Response: {"status":"ok"}
```

---

### 2. Run with Docker Compose (Local PostgreSQL)

To start EMS alongside PostgreSQL 16:

```bash
docker compose up -d --build
```

---

### 3. Production Deployment with Traefik & TLS

EMS includes a turnkey production configuration featuring Traefik v3, automated Let's Encrypt TLS certificates, and container hardening:

1. **Prepare production environment file**:
   ```bash
   cp .env.production.example .env
   ```

2. **Generate secure keys**:
   ```bash
   # Master Encryption Key (32-byte / 64 hex characters)
   openssl rand -hex 32

   # Admin API Key
   openssl rand -base64 32

   # PostgreSQL Password
   openssl rand -hex 16
   ```

3. **Deploy the production stack**:
   ```bash
   docker compose -f compose.prod.yaml up -d --build
   ```

Refer to [`docs/PRODUCTION.md`](./docs/PRODUCTION.md) for full operational, backup, and disaster recovery procedures.

---

## API Usage Examples

All mutating endpoints accept an optional `Idempotency-Key` header and require authentication via `X-API-Key` or `Authorization: Bearer <key>`.

### 1. Register a Cloudflare Account

```bash
curl -X POST http://localhost:8080/api/v1/accounts \
  -H "X-API-Key: ems-admin-secret-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Bariskode Production",
    "cloudflare_account_id": "cf_account_12345",
    "api_token": "cf_token_secret_xyz"
  }'
```

> **Security Note:** The `api_token` is immediately encrypted using AES-256-GCM before database insertion and is never returned in API responses or logs.

---

### 2. Register & Verify a Destination Address

```bash
curl -X POST http://localhost:8080/api/v1/destinations \
  -H "X-API-Key: ems-admin-secret-key" \
  -H "Content-Type: application/json" \
  -d '{
    "account_id": "cf_account_12345",
    "email": "ops@gmail.com"
  }'
```

---

### 3. Import a Zone from Cloudflare

```bash
curl -X POST http://localhost:8080/api/v1/zones/import \
  -H "X-API-Key: ems-admin-secret-key" \
  -H "Content-Type: application/json" \
  -d '{
    "account_id": "cf_account_12345",
    "provider_zone_id": "cf_zone_67890"
  }'
```

---

### 4. Create an Explicit Forwarding Rule

```bash
curl -X POST http://localhost:8080/api/v1/zones/{zone_id}/rules \
  -H "X-API-Key: ems-admin-secret-key" \
  -H "Idempotency-Key: req-create-rule-001" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Support Forwarder",
    "matcher_type": "literal",
    "matcher_field": "to",
    "matcher_value": "support@bariskode.com",
    "action_type": "forward",
    "destination": "ops@gmail.com",
    "enabled": true
  }'
```

---

### 5. Configure Dedicated Catch-All

```bash
curl -X PUT http://localhost:8080/api/v1/zones/{zone_id}/catch-all \
  -H "X-API-Key: ems-admin-secret-key" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "action_type": "forward",
    "destination": "ops@gmail.com"
  }'
```

---

### 6. Trigger Drift Detection & Synchronization

```bash
curl -X POST http://localhost:8080/api/v1/zones/{zone_id}/sync \
  -H "X-API-Key: ems-admin-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"direction": "drift_check"}'
```

**Example Drift Report:**
```json
{
  "data": {
    "run_id": "syn_8f21bc90",
    "status": "success",
    "diffs": [
      {
        "status": "MATCHED",
        "resource_type": "routing_settings",
        "identifier": "enabled"
      },
      {
        "status": "CHANGED",
        "resource_type": "rule",
        "identifier": "support@bariskode.com",
        "details": "Rule destination, action, or enabled status differs"
      }
    ],
    "changes_count": 0,
    "error_count": 0
  },
  "meta": {
    "request_id": "req_841da94d33458117"
  }
}
```

---

## Testing & Verification

EMS adheres to strict test-driven reliability. Run the test suite:

```bash
# Run all unit and integration tests with race detector
make test

# Run Go static analysis
go vet ./...

# Build binary
make build
```

---

## Documentation Index

- [`docs/PRD.md`](./docs/PRD.md) — Product requirements, scope, and functional specs
- [`docs/DESIGN.md`](./docs/DESIGN.md) — System architecture, layering, and failure modes
- [`docs/SPECS.md`](./docs/SPECS.md) — API contracts, Cloudflare endpoints, and data schemas
- [`docs/PRODUCTION.md`](./docs/PRODUCTION.md) — Production operations, hardening, and backup runbook
- [`docs/PROGRESS.md`](./docs/PROGRESS.md) — Feature implementation and verification log
- [`api/openapi.yaml`](./api/openapi.yaml) — Complete OpenAPI 3.1 contract specification

---

## Development Status

- **Current Version:** `0.1.0`
- **Milestone:** **M0-M8 Complete (MVP & Production Hardening Ready)**
- **Next Milestone:** M9 (SvelteKit Web Dashboard)

---

## License

This project is licensed under the [MIT License](./LICENSE).
