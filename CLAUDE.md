# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Status

This repository currently contains **only planning documentation — no code has been written yet** (no `go.mod`, no source files, no commits beyond the initial docs). Before implementing anything, read the docs in `docs/` — they are the design authority for this project and any implementation must conform to the decisions recorded there.

- `README.md` — project overview and elevator pitch
- `docs/PRD.md` — product requirements, scope, non-goals, functional requirements (FR-001..FR-010)
- `docs/SPECS.md` — technical specs: Cloudflare API endpoints, domain model, REST API surface, sync algorithm, security requirements
- `docs/DESIGN.md` — architecture, layering, provider adapter pattern, state ownership, failure handling
- `docs/PLAN.md` — phased implementation plan (Phase 0 through Phase 13) and milestones (M0-M9)
- `docs/MEMORY.md` — condensed persistent decisions (the fastest file to re-read for context)
- `docs/PROGRESS.md` — current implementation status; update this when work actually lands

When starting implementation work, update `docs/PROGRESS.md` to reflect real status — do not mark anything complete based on code compiling alone (see "Verification Rules" in that file: a feature is only done when implementation + tests + verification + documentation all exist).

## What This Project Is

Email Management Service (EMS) is a self-hosted control plane for centrally managing **Cloudflare Email Routing** across multiple domains — zones, destination addresses, explicit routing rules, catch-all rules, synchronization, and audit history — exposed via a REST API (and later a web dashboard).

**It is not a mailbox provider.** No IMAP/POP3/webmail/SMTP. Cloudflare Email Sending (outbound) is an explicitly separate, deferred capability from Email Routing (inbound) — never conflate the two.

## Core Architectural Decisions (from docs/DESIGN.md and docs/MEMORY.md)

1. **Modular monolith first.** Do not introduce microservices (`ems-api`, `ems-sync-worker`, `ems-provider-cloudflare`, etc.) until operational requirements justify extraction.
2. **Provider abstraction is mandatory.** Handlers must never call Cloudflare directly — all Cloudflare access goes through an `EmailProvider` interface (specified in `docs/SPECS.md` §3), with Cloudflare-specific request/response mapping confined to `internal/provider/cloudflare/`. This is what makes provider logic unit-testable without real API calls.
3. **Catch-all is a first-class resource, not a generic rule.** Model it as `Zone.CatchAll`, separate from `Zone.ExplicitRules[]`, and always use Cloudflare's dedicated catch-all endpoint (`GET/PUT /zones/{zone_id}/email/routing/rules/catch_all`), never a rule with `matcher=all`.
4. **State ownership split:** Cloudflare is authoritative for actual infrastructure state (zone IDs, routing state, rule IDs, destination verification). EMS owns desired/application state, local IDs, audit history, sync history, and credential references. Never assume local state equals provider state — that's what sync/drift detection is for.
5. **Destinations are account-level**, not zone-level, and can be reused across zones within the same Cloudflare account — keep them modeled separately from zones.
6. **Read-after-write for critical mutations.** On provider timeout, do not commit local "success" state — record the operation as failed/uncertain and reconcile via a follow-up GET rather than assuming success.
7. **Layering is strict:** HTTP Handler → Application Service → Domain Model → Repository/Provider Interface → PostgreSQL/Cloudflare. No Cloudflare-specific logic leaks upward past the provider adapter.

## Intended Repository Layout (not yet created)

```text
cmd/ems/main.go
internal/
  auth/ config/ domain/ provider/ (provider.go + cloudflare/)
  routing/ destination/ sync/ audit/ storage/ httpapi/ observability/
migrations/
api/openapi.yaml
docs/
tests/
Dockerfile, compose.yaml, Makefile, go.mod
```

## Intended Technology Stack

| Component | Technology |
|---|---|
| Backend | Go (1.24+ target) |
| Database | PostgreSQL 16+ (SQL migrations) |
| API | REST/JSON, versioned under `/api/v1` |
| Contract | OpenAPI 3.1 |
| Deployment | Docker / Docker Compose, Traefik reverse proxy |
| Future UI | SvelteKit + TypeScript |

## Build/Test Commands (target, per docs/PLAN.md Phase 0 exit criteria)

Once the Go module exists, the expected baseline commands are:

```text
go test ./...
go vet ./...
docker compose up
GET /healthz → 200
```

A `Makefile` is planned to wrap these; until it exists, use the raw Go/Docker commands above.

## Key Behavioral Rules When Implementing

- Never request broad `Account:Edit` or `Zone:Edit` Cloudflare token scopes when narrower permissions suffice (see `docs/SPECS.md` §9).
- Never log or persist Cloudflare API tokens or the master encryption key in plaintext; secrets are encrypted at rest and decrypted only in-memory for the duration of a provider call.
- Every mutating endpoint should support an `Idempotency-Key` header, scoped by `actor + endpoint + key`.
- Every mutation must produce an audit event (actor, request ID, operation, resource, before/after state, provider result, status/error) — but audit records themselves must never contain tokens or secrets.
- Provider errors must be normalized into stable EMS error codes, not passed through raw.
- Unit tests for the Cloudflare provider must use a mocked HTTP transport, never real API calls; end-to-end tests must use a dedicated Cloudflare test account/zone and must never run destructive operations against production domains.
