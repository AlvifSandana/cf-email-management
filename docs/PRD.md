# Email Management Service — Product Requirements Document

**Version:** 0.1.0  
**Status:** Draft / Foundation  
**Date:** 2026-10-02  
**Product:** Email Management Service (EMS)

---

## 1. Executive Summary

Email Management Service (EMS) is a self-hosted service for centrally managing custom email routing across domains hosted in Cloudflare.

The primary objective is to replace repetitive Cloudflare Dashboard operations with a single API and, later, a lightweight web dashboard.

EMS manages:

- Cloudflare zones/domains
- Email Routing enablement/status
- Verified destination addresses
- Explicit custom-address routing rules
- Catch-all routing rules
- Cloudflare Worker-based email actions
- Synchronization between local state and Cloudflare
- Audit history
- Provider/API credentials
- Health and synchronization status

The initial product is **routing management**, not a mailbox provider. EMS does not provide IMAP/POP3 inboxes.

---

## 2. Problem Statement

When multiple domains are registered or managed through Cloudflare, configuring email aliases manually becomes repetitive and error-prone.

Typical operations include:

1. Discovering Cloudflare zones.
2. Enabling Email Routing.
3. Creating/verifying destination addresses.
4. Creating aliases such as `hello@domain.com`.
5. Configuring forwarding destinations.
6. Configuring catch-all behavior.
7. Updating or deleting rules.
8. Checking whether Cloudflare and the local intended state are consistent.

EMS centralizes those operations behind a provider abstraction and API.

---

## 3. Goals

### 3.1 Primary Goals

- Manage multiple Cloudflare domains from one service.
- Manage explicit email routing rules.
- Manage catch-all rules.
- Manage account-level destination addresses.
- Detect and report drift between EMS state and Cloudflare.
- Provide safe, idempotent APIs.
- Keep provider credentials out of normal application data.
- Provide an audit trail for changes.
- Support future web UI without redesigning the backend.

### 3.2 Secondary Goals

- Support Cloudflare Workers as routing actions.
- Provide synchronization/reconciliation.
- Support dry-run operations.
- Provide operational health endpoints.
- Support multiple Cloudflare accounts.
- Make the provider layer replaceable.

---

## 4. Non-Goals

The following are explicitly outside MVP:

- Full mailbox hosting.
- IMAP/POP3 server.
- Webmail.
- User mailbox storage.
- Spam filtering engine.
- SMTP server implementation.
- Email marketing platform.
- Bulk outbound email system.
- Domain registrar management.
- DNS management unrelated to Email Routing.
- Automatic ownership of arbitrary third-party email providers.

Cloudflare Email Sending may be integrated later as a separate outbound capability.

---

## 5. Target Users

### Primary

The owner/operator of multiple Cloudflare-managed domains who wants centralized email routing automation.

### Secondary

- Small development teams.
- SaaS operators.
- Internal IT administrators.
- DevOps/SRE operators.
- Agencies managing multiple domains.

---

## 6. Core Use Cases

### UC-01 — Discover domains

The operator connects a Cloudflare API token and imports eligible zones.

### UC-02 — Enable Email Routing

The operator enables Email Routing for a selected zone.

### UC-03 — Add destination address

The operator adds a destination such as:

`ops@example@gmail.com`

Cloudflare sends a verification email. EMS records verification status.

### UC-04 — Create custom alias

Example:

`support@bariskode.com -> ops@example@gmail.com`

### UC-05 — Configure catch-all

Example:

`*@bariskode.com -> inbox@example@gmail.com`

Catch-all must be modeled as a dedicated rule because Cloudflare exposes a dedicated catch-all API.

### UC-06 — Disable catch-all

The operator disables catch-all without deleting explicit rules.

### UC-07 — Synchronize

EMS compares local intended state with Cloudflare's current state and reports differences.

### UC-08 — Audit

Every mutating operation records who/what initiated it, target resource, previous state, desired state, provider response, and result.

---

## 7. Product Principles

1. **Provider API is the source of actual infrastructure state.**
2. **EMS database stores desired/application state and operational metadata.**
3. **Every mutation must be idempotent where practical.**
4. **Destructive actions require explicit intent.**
5. **Secrets are never returned from normal APIs.**
6. **Cloudflare identifiers are persisted to avoid name-based ambiguity.**
7. **Catch-all is first-class.**
8. **Sync is observable and auditable.**
9. **No hidden provider-specific behavior in domain/business logic.**
10. **A provider failure must not corrupt local state.**

---

## 8. MVP Scope

### Included

- Authentication for EMS operators.
- Cloudflare account configuration.
- Zone discovery.
- Zone import.
- Email Routing status.
- Destination address CRUD.
- Destination verification status.
- Explicit routing rule CRUD.
- Catch-all GET/PUT.
- Rule synchronization.
- Drift detection.
- Audit log.
- Health endpoints.
- OpenAPI documentation.
- Docker deployment.

### Deferred

- Web dashboard.
- Cloudflare Workers deployment management.
- Email Sending integration.
- Multi-user RBAC beyond basic admin/operator.
- Multiple providers.
- Notifications.
- Scheduled reconciliation.

---

## 9. Functional Requirements

### FR-001 Cloudflare Accounts

EMS SHALL support storing multiple Cloudflare account configurations.

### FR-002 Zone Discovery

EMS SHALL be able to retrieve zones from Cloudflare and import selected zones.

### FR-003 Email Routing Status

EMS SHALL display whether Email Routing is enabled for a zone.

### FR-004 Destination Addresses

EMS SHALL support:

- list
- create
- read
- update
- delete
- verification status

Destination addresses are account-level in Cloudflare and may be reused by domains within the same Cloudflare account.

### FR-005 Explicit Routing Rules

EMS SHALL support:

- list
- create
- update
- delete
- enable/disable

MVP matcher:

- literal `to` address

MVP actions:

- forward
- drop

Future action:

- worker

### FR-006 Catch-All

EMS SHALL support:

- get catch-all
- enable/update catch-all
- disable catch-all
- forward
- drop
- worker in future

Catch-all SHALL NOT be represented merely as a normal explicit rule.

### FR-007 Sync

EMS SHALL support:

- provider -> local import
- local desired state -> provider
- drift detection
- sync result reporting

### FR-008 Audit

Every mutation SHALL generate an audit event.

### FR-009 Idempotency

Mutating API endpoints SHOULD support idempotency keys.

### FR-010 Error Handling

Provider errors SHALL be normalized into stable EMS error codes.

---

## 10. Non-Functional Requirements

### Performance

- API p95 under 300 ms for local-only operations under normal load.
- Provider operations are allowed to exceed this because they depend on Cloudflare API latency.

### Reliability

- No partial local commit after failed provider mutation.
- Sync operations must be retryable.

### Security

- TLS in production.
- API tokens encrypted at rest.
- Secret values excluded from logs.
- Structured audit events.
- Rate limiting.
- Request authentication.

### Maintainability

- Go backend.
- PostgreSQL.
- Provider interface.
- OpenAPI contract.
- Unit/integration tests.

---

## 11. Success Criteria

MVP is successful when an operator can:

1. Connect Cloudflare.
2. Discover domains.
3. Enable Email Routing.
4. Add a verified destination.
5. Create `support@domain`.
6. Configure catch-all.
7. Modify/delete rules.
8. Sync and identify drift.
9. Review an audit trail.

---

## 12. Future Product Direction

Potential evolution:

```text
EMS
├── Cloudflare Email Routing
├── Cloudflare Email Sending
├── Cloudflare Workers Email
├── Provider abstraction
├── Notifications
├── Dashboard
└── Multi-account / multi-provider management
```
