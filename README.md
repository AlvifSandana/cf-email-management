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
- **Embedded Web Dashboard**: Modern responsive UI served natively at `GET /` and `GET /dashboard` for managing rules, destinations, catch-all, and monitoring drift in real-time.
- **Dedicated CLI Utility (`ems-cli`)**: Intuitive command-line interface for automating accounts, destinations, rules, catch-all, and sync workflows.
- **Dedicated First-Class Catch-All**: Models catch-all rules natively using Cloudflare's dedicated `/catch_all` endpoints, distinct from generic routing rules.
- **Verification-Gated Forwarding**: Prevents misconfigurations by verifying that destination addresses are confirmed before routing rules can forward mail to them.
- **Bi-Directional Sync & Drift Detection**: Detects divergence between local desired state and remote infrastructure (`MATCHED`, `LOCAL_ONLY`, `REMOTE_ONLY`, `CHANGED`).
- **Scheduled Background Reconciliation**: Automated background worker (`internal/sync/worker.go`) periodically polling zones for configuration drift with configurable callbacks.
- **Multi-Channel Webhook Alerting**: Instant notifications via **Slack**, **Discord**, **Telegram**, or generic JSON webhooks on drift detection or destination verification.
- **Cloudflare Worker Actions & Deployment**: Support for Cloudflare Worker email routing actions, plus automated script inspection, upload, and deletion (`internal/provider/cloudflare/workers.go`).
- **Cloudflare Outbound Email Sending**: Clean client abstraction for Cloudflare's outbound sending API with error normalization into domain errors (`internal/email/sending`).
- **Multi-Provider Architecture (AWS SES Adapter)**: Pluggable provider abstraction with full AWS SES adapter (`internal/provider/ses`) for domains, receipt rules, and verified identities.
- **Multi-User RBAC & OIDC Authentication**: Role-based access control (Admin, Operator, Viewer) with permission enforcement and OIDC JWT bearer token verification (`internal/auth/rbac`, `internal/auth/oidc.go`).
- **Zero-Trust Secret Security**: Provider API tokens are encrypted at rest using AES-256-GCM with a 32-byte master key. Sensitive tokens and passwords are automatically redacted from audit logs.
- **Idempotent Mutations**: Supports `Idempotency-Key` headers on mutating requests, preventing duplicate rule creation and unintended side-effects.
- **Production-Grade Hardening**: Dockerized with a non-root user (`ems:ems`), read-only root container filesystem, Traefik automated TLS, and sliding-window rate limiting.
- **CI/CD Quality Gate**: Production GitHub Actions pipeline running tests, race detection, formatting checks, linters, container builds, and Trivy security scans.
- **Observability Native**: Out-of-the-box `/healthz`, `/readyz`, and Prometheus `/metrics` endpoints.

---

## Important Distinction: Routing vs Mailboxes

> [!NOTE]
> **EMS is an email routing control plane, not a mailbox host.**

- **Supported:** Forwarding custom aliases (e.g., `support@bariskode.com` &rarr; `team@gmail.com`), catch-all routing (`*@bariskode.com`), drop actions, and worker scripts.
- **Out of Scope:** IMAP/POP3 servers, webmail interfaces, and local mailbox storage.

---

## Repository Structure

```text
email-management-service/
├── .github/
│   └── workflows/ci.yml     # Production CI/CD workflow (test, lint, docker, trivy)
├── cmd/
│   ├── ems/                 # EMS daemon entry point (bootstrapping & graceful shutdown)
│   └── ems-cli/             # Dedicated CLI tool for terminal operations
├── internal/
│   ├── audit/               # Audit service with automatic secret redaction
│   ├── auth/                # AES-256-GCM encryption, API keys, RBAC & OIDC JWT validation
│   │   └── rbac/            # Role hierarchy (Admin, Operator, Viewer) & permission middleware
│   ├── config/              # Environment configuration loader
│   ├── destination/         # Destination address management & verification checking
│   ├── domain/              # Core domain models, enums, and normalized errors
│   ├── email/
│   │   └── sending/         # Cloudflare outbound email sending client
│   ├── httpapi/             # REST API handlers, routing, idempotency & security middleware
│   ├── notification/        # Webhook alerting service (Slack, Discord, Telegram, Generic)
│   ├── observability/       # Health probes (/healthz, /readyz) & Prometheus metrics
│   ├── provider/
│   │   ├── cloudflare/      # Cloudflare API v4 adapter & Worker script deployment
│   │   ├── mock/            # In-memory mock provider for testing
│   │   ├── ses/             # Multi-provider AWS SES adapter
│   │   └── provider.go      # Abstract EmailProvider interface
│   ├── routing/             # Zone discovery, explicit routing rules & catch-all services
│   ├── storage/             # Repository interfaces with Postgres & Memory implementations
│   └── sync/                # Sync engine, diff calculator & background reconciliation worker
├── migrations/              # Embedded SQL migrations (auto-applied on startup)
├── scripts/                 # Automated backup & restore runbooks (Bash & PowerShell)
├── web/                     # Embedded single-page Web Dashboard (HTML5, Tailwind, JS)
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

## Web Dashboard & CLI Management Tool

### 1. Web Dashboard (Built-in SPA)

EMS bundles an embedded responsive Single-Page Dashboard (`web/index.html`) directly into the Go binary. No extra build steps or node dependencies are required.

- **URL:** Open `http://localhost:8080/` or `http://localhost:8080/dashboard` in your browser.
- **Capabilities:**
  - View configured accounts and zones.
  - Inspect, create, and modify explicit routing rules.
  - Enable and update dedicated Catch-All routing.
  - Verify and register destination forwarding addresses.
  - Trigger one-click drift detection and view real-time synchronization diffs.

---

### 2. Dedicated CLI Tool (`cmd/ems-cli`)

For terminal-first workflows, CI automation, and scripting:

```bash
# Build the CLI utility
go build -o bin/ems-cli ./cmd/ems-cli

# Set environment variables (or pass --api-key and --url flags)
export EMS_API_URL=http://localhost:8080
export EMS_API_KEY=ems-admin-secret-key

# List registered zones
./bin/ems-cli zones list

# Trigger zone drift check
./bin/ems-cli sync run <zone_id> drift_check

# Pull remote Cloudflare configuration to local DB
./bin/ems-cli sync run <zone_id> pull

# List routing rules for a zone
./bin/ems-cli rules list <zone_id>

# Configure dedicated catch-all forwarding
./bin/ems-cli catch-all set <zone_id> forward team@gmail.com true

# List destination addresses
./bin/ems-cli destinations list
```

---

## Testing & Verification

EMS adheres to strict test-driven reliability. Run the test suite:

```bash
# Run all unit and integration tests
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
- **Status:** **All Roadmap Milestones Complete (M0-M9 & Advanced Roadmap)**
  - Core MVP & Persistence (M0–M8)
  - Web Dashboard & Management CLI (M9)
  - Scheduled Background Reconciliation Worker
  - Multi-Channel Notification Webhook Alerting (Slack, Discord, Telegram, Generic)
  - Cloudflare Outbound Email Sending Client
  - Multi-User RBAC & OIDC JWT Authentication
  - Cloudflare Worker Script Deployment Management
  - Multi-Provider AWS SES Adapter
  - Production GitHub Actions CI/CD Quality Pipeline

---

## License

This project is licensed under the [MIT License](./LICENSE).

