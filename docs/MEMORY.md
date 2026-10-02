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

## 11. Future Direction

Possible future modules:

```text
Email Routing
Email Sending
Workers
Notifications
Dashboard
Multi-provider
RBAC
OIDC
```

Email Sending remains logically separate from Email Routing.

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

Cloudflare Email Service configuration:

https://developers.cloudflare.com/email-service/configuration/email-routing-addresses/

---

## 13. Open Decisions

These are intentionally not finalized:

- OIDC vs API key as primary authentication.
- Whether desired state is stored separately from observed state.
- Whether to use a job queue for sync.
- Dashboard implementation details.
- Notification channels.
- Multi-provider roadmap.
- Secret backend: encrypted PostgreSQL vs external secret manager.
