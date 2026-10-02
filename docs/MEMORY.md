# Email Management Service — Project Memory

**Version:** 0.1.0  
**Last Updated:** 2026-10-02

This file is the persistent project memory intended to help future development sessions recover important decisions without rereading the entire documentation set.

---

## 1. Project Identity

**Name:** Email Management Service  
**Short name:** EMS

Purpose:

> Centrally manage Cloudflare Email Routing across multiple domains through an API and eventually a dashboard.

---

## 2. Core Decision

EMS is primarily a **routing management control plane**, not a mailbox provider.

It manages:

- domains/zones
- routing rules
- catch-all
- destination addresses
- synchronization
- audit

It does not provide:

- IMAP
- POP3
- webmail
- mailbox storage

---

## 3. Provider

Initial provider:

```text
Cloudflare
```

Provider abstraction is mandatory.

Reason:

Future providers may be added without rewriting business logic.

---

## 4. Cloudflare Facts

Important current API facts:

- Routing rules have CRUD endpoints.
- Catch-all has dedicated GET/PUT endpoints.
- Destination addresses are account-level.
- Destination addresses must be verified before forwarding.
- Routing actions currently include forward, drop, and worker.
- Catch-all uses matcher type `all`.

---

## 5. Catch-All Decision

Catch-all is a first-class EMS resource.

Do not model it only as a normal explicit rule.

```text
Zone
├── Explicit Rules[]
└── CatchAll
```

---

## 6. Data Ownership

Cloudflare:

```text
actual infrastructure state
```

EMS:

```text
application state
desired state
audit
sync metadata
credentials references
```

Provider state must always be treated as authoritative for actual infrastructure.

---

## 7. Technology Decision

Backend:

```text
Go
```

Database:

```text
PostgreSQL
```

API:

```text
REST / JSON
OpenAPI 3.1
```

Deployment:

```text
Docker
Docker Compose
Traefik or equivalent reverse proxy
```

Future dashboard:

```text
SvelteKit + TypeScript
```

---

## 8. Architectural Decision

Start as a modular monolith.

Do not introduce microservices until operational requirements justify extraction.

---

## 9. Security Decisions

- Use Cloudflare API Tokens.
- Prefer least privilege.
- Encrypt provider credentials at rest.
- Never log secrets.
- Audit all mutations.
- Use request IDs.
- Use idempotency keys.
- Use rate limiting.
- Use TLS in production.

---

## 10. Sync Philosophy

Sync must be explicit and observable.

Possible states:

```text
MATCHED
LOCAL_ONLY
REMOTE_ONLY
CHANGED
UNKNOWN
```

Provider mutation should be followed by read-after-write for important operations.

---

## 11. Implemented Modules

```text
Email Routing            100% (Explicit rules + Dedicated Catch-All)
Destination Management   100% (Enforced verification check)
Sync & Drift Engine      100% (Diff calculation: MATCHED, CHANGED, etc.)
Reconciliation Worker    100% (Scheduled background ticker with callbacks)
Email Sending            100% (Cloudflare Outbound Send API Client)
Worker Management        100% (Worker routing actions + Script upload/delete)
Notifications            100% (Slack, Discord, Telegram, Generic Webhooks)
Web Dashboard            100% (Embedded Single-Page App at / and /dashboard)
CLI Management Tool      100% (cmd/ems-cli terminal utility)
Multi-Provider           100% (AWS SES Adapter + Cloudflare Adapter)
RBAC & OIDC Auth         100% (Admin, Operator, Viewer + OIDC JWT validation)
Production Hardening     100% (Traefik TLS, Rate limiting, Security headers)
CI/CD Pipeline           100% (GitHub Actions test, lint, docker, trivy)
```

Email Sending remains logically separated under `internal/email/sending/`.

---

## 12. Important References

Cloudflare Email Routing API:
https://developers.cloudflare.com/api/resources/email_routing/

Cloudflare Routing Rules:
https://developers.cloudflare.com/api/resources/email_routing/subresources/rules/

Cloudflare Catch-All:
https://developers.cloudflare.com/api/resources/email_routing/subresources/rules/subresources/catch_alls/

Cloudflare Destination Addresses:
https://developers.cloudflare.com/api/resources/email_routing/subresources/addresses/

Cloudflare Email Sending REST API:
https://developers.cloudflare.com/api/resources/email_sending/

Cloudflare Workers Scripts API:
https://developers.cloudflare.com/api/resources/workers/subresources/scripts/

---

## 13. Resolved Technical Decisions

- **Authentication Architecture:** Combined hybrid model supporting static `X-API-Key` (defaults to Admin role) and OIDC JWT Bearer tokens with RBAC role claims (`Admin`, `Operator`, `Viewer`).
- **Sync & Drift Execution:** Per-zone concurrency locking via in-memory mutexes, scheduled via background ticker worker (`internal/sync/worker.go`) with non-blocking callback alerts.
- **Multi-Channel Alerting:** Centralized `notification.Service` supporting Slack block attachments, Discord embeds, Telegram markdown, and Generic JSON payloads.
- **Multi-Provider Strategy:** Provider abstraction `EmailProvider` implemented for Cloudflare API v4 and AWS SES (domain identities, receipt rules, and verified identities).
- **Secrets Backend:** Encrypted PostgreSQL using AES-256-GCM with a 32-byte master key passed via `EMS_MASTER_KEY`.
- **Git Multi-Key SSH:** Remote `github-AlvifSandana` alias in `~/.ssh/config` must always be used for `AlvifSandana/cf-email-management.git` to bind to `id_ed25519_github_AlvifSandana`.
- **Go Toolchain Target:** Target `go 1.24` in `go.mod` to ensure compatibility across local compilers, GitHub Actions runners, and static analysis linters (`golangci-lint`, `staticcheck`).

