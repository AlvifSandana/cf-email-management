# Email Management Service — Technical Specifications

**Version:** 0.1.0  
**Status:** Draft  
**Date:** 2026-10-02

---

## 1. Technology Baseline

### Backend

- Go 1.24+ target
- REST API
- `net/http` or a lightweight HTTP router
- PostgreSQL 16+
- SQL migrations
- Structured logging
- OpenTelemetry-ready interfaces

### Deployment

- Docker
- Docker Compose for local/small deployment
- Reverse proxy such as Traefik
- HTTPS required in production

### API

- REST/JSON
- OpenAPI 3.1
- Versioned prefix: `/api/v1`

---

## 2. Cloudflare Integration

Cloudflare's current Email Routing API exposes:

### Email Routing settings

```http
GET   /zones/{zone_id}/email/routing
PATCH /zones/{zone_id}/email/routing
PUT   /zones/{zone_id}/email/routing
```

### Routing rules

```http
GET    /{accounts_or_zones}/{account_or_zone_id}/email/routing/rules
GET    /zones/{zone_id}/email/routing/rules/{rule_identifier}
POST   /zones/{zone_id}/email/routing/rules
PUT    /zones/{zone_id}/email/routing/rules/{rule_identifier}
DELETE /zones/{zone_id}/email/routing/rules/{rule_identifier}
```

### Catch-all

```http
GET /zones/{zone_id}/email/routing/rules/catch_all
PUT /zones/{zone_id}/email/routing/rules/catch_all
```

### Destination addresses

```http
GET    /accounts/{account_id}/email/routing/addresses
GET    /accounts/{account_id}/email/routing/addresses/{id}
POST   /accounts/{account_id}/email/routing/addresses
PATCH  /accounts/{account_id}/email/routing/addresses/{id}
DELETE /accounts/{account_id}/email/routing/addresses/{id}
```

Cloudflare destination addresses are account-level and must be verified before they can be used for forwarding.

---

## 3. Provider Interface

The application MUST NOT call Cloudflare directly from handlers.

Suggested abstraction:

```go
type EmailProvider interface {
    ListZones(ctx context.Context) ([]Zone, error)

    GetEmailRoutingSettings(
        ctx context.Context,
        zoneID string,
    ) (RoutingSettings, error)

    UpdateEmailRoutingSettings(
        ctx context.Context,
        zoneID string,
        settings RoutingSettingsUpdate,
    ) error

    ListDestinationAddresses(
        ctx context.Context,
        accountID string,
    ) ([]DestinationAddress, error)

    CreateDestinationAddress(
        ctx context.Context,
        accountID string,
        email string,
    ) (DestinationAddress, error)

    UpdateDestinationAddress(
        ctx context.Context,
        accountID, addressID string,
        email string,
    ) (DestinationAddress, error)

    DeleteDestinationAddress(
        ctx context.Context,
        accountID, addressID string,
    ) error

    ListRules(
        ctx context.Context,
        zoneID string,
    ) ([]RoutingRule, error)

    CreateRule(
        ctx context.Context,
        zoneID string,
        rule CreateRoutingRule,
    ) (RoutingRule, error)

    UpdateRule(
        ctx context.Context,
        zoneID, ruleID string,
        rule UpdateRoutingRule,
    ) (RoutingRule, error)

    DeleteRule(
        ctx context.Context,
        zoneID, ruleID string,
    ) error

    GetCatchAll(
        ctx context.Context,
        zoneID string,
    ) (CatchAllRule, error)

    UpdateCatchAll(
        ctx context.Context,
        zoneID string,
        rule UpdateCatchAll,
    ) (CatchAllRule, error)
}
```

---

## 4. Domain Model

### CloudflareAccount

```text
id
name
cloudflare_account_id
credential_ref
status
created_at
updated_at
```

### Zone

```text
id
provider_account_id
provider_zone_id
name
status
email_routing_enabled
last_synced_at
created_at
updated_at
```

### DestinationAddress

```text
id
provider_account_id
provider_address_id
email
verified_at
status
created_at
updated_at
```

### RoutingRule

```text
id
zone_id
provider_rule_id
name
matcher_type
matcher_field
matcher_value
action_type
destination
enabled
source
created_at
updated_at
```

### CatchAllRule

```text
id
zone_id
provider_rule_id
action_type
destination
enabled
source
last_synced_at
created_at
updated_at
```

### SyncRun

```text
id
zone_id
direction
status
started_at
finished_at
changes_count
error_count
summary
```

### AuditEvent

```text
id
actor_type
actor_id
operation
resource_type
resource_id
request_id
before_json
after_json
provider_response_json
status
error_code
created_at
```

---

## 5. Database Constraints

### Destination

- `email` must be unique within provider account.
- `provider_address_id` unique.

### Zone

- `(provider_account_id, provider_zone_id)` unique.
- `name` unique within provider account.

### Rule

- `provider_rule_id` unique per zone.
- Explicit literal matcher should not duplicate an existing normalized address.

### Catch-all

- Exactly one logical catch-all record per zone.

---

## 6. REST API

### Accounts

```http
GET  /api/v1/accounts
POST /api/v1/accounts
GET  /api/v1/accounts/{id}
PUT  /api/v1/accounts/{id}
DELETE /api/v1/accounts/{id}
```

### Zones

```http
GET  /api/v1/zones
POST /api/v1/zones/import
GET  /api/v1/zones/{id}
POST /api/v1/zones/{id}/sync
```

### Routing

```http
GET  /api/v1/zones/{id}/routing
PATCH /api/v1/zones/{id}/routing
```

### Rules

```http
GET    /api/v1/zones/{id}/rules
POST   /api/v1/zones/{id}/rules
GET    /api/v1/zones/{id}/rules/{ruleID}
PUT    /api/v1/zones/{id}/rules/{ruleID}
DELETE /api/v1/zones/{id}/rules/{ruleID}
```

### Catch-all

```http
GET /api/v1/zones/{id}/catch-all
PUT /api/v1/zones/{id}/catch-all
```

Example:

```json
{
  "enabled": true,
  "action": {
    "type": "forward",
    "destination": "dest_123"
  }
}
```

### Destinations

```http
GET    /api/v1/destinations
POST   /api/v1/destinations
GET    /api/v1/destinations/{id}
PATCH  /api/v1/destinations/{id}
DELETE /api/v1/destinations/{id}
```

### Audit

```http
GET /api/v1/audit-events
GET /api/v1/sync-runs
```

---

## 7. API Response Contract

Success:

```json
{
  "data": {},
  "meta": {
    "request_id": "req_..."
  }
}
```

Error:

```json
{
  "error": {
    "code": "DESTINATION_NOT_VERIFIED",
    "message": "The destination address has not been verified.",
    "request_id": "req_..."
  }
}
```

---

## 8. Rule Semantics

### Explicit rule

```text
matcher:
    type = literal
    field = to
    value = support@bariskode.com

action:
    type = forward
    destination = verified destination
```

### Catch-all

```text
matcher:
    type = all

action:
    type = forward
    destination = verified destination
```

Catch-all must use the dedicated Cloudflare catch-all endpoint.

---

## 9. Security

### Cloudflare Token

Use API Tokens, not the legacy global API key, wherever possible.

Minimum intended permissions should be narrowed to the operations required by the installation.

At minimum, depending on deployment behavior:

- Zone read access for zone discovery.
- Email Routing Rules read/write.
- Email Routing Addresses read/write.
- Email Routing settings access as required.

Never request broad `Account:Edit` or `Zone:Edit` privileges when narrower permissions are sufficient.

### Secret Storage

Recommended:

```text
DB stores:
    encrypted credential

Application memory:
    decrypted credential only when making provider calls

Logs:
    NEVER contain token
```

---

## 10. Idempotency

Every mutating endpoint should accept:

```http
Idempotency-Key: <client-generated-key>
```

The key is scoped by:

```text
actor + endpoint + key
```

Responses are cached for a bounded period.

---

## 11. Concurrency

Use optimistic locking for local resources.

Provider synchronization should use per-zone locks:

```text
zone A sync ── lock A
zone B sync ── lock B
```

Do not serialize all domains globally.

---

## 12. Sync Algorithm

```text
1. Load local zone.
2. Fetch Cloudflare routing settings.
3. Fetch explicit rules.
4. Fetch catch-all.
5. Fetch account destinations.
6. Normalize provider state.
7. Compare with local state.
8. Produce diff.
9. Apply requested direction.
10. Record sync result.
11. Record audit events.
```

---

## 13. Drift Model

Possible drift:

```text
LOCAL_ONLY
REMOTE_ONLY
CHANGED
MATCHED
UNKNOWN
```

Example:

```text
support@domain.com

LOCAL:
  -> support@gmail.com

CLOUDFLARE:
  -> ops@gmail.com

RESULT:
  CHANGED
```

---

## 14. Observability

Endpoints:

```http
GET /healthz
GET /readyz
GET /metrics
```

Metrics:

```text
ems_provider_requests_total
ems_provider_request_duration_seconds
ems_sync_runs_total
ems_sync_errors_total
ems_rule_mutations_total
ems_api_requests_total
```

---

## 15. Cloudflare API References

Official documentation:

- Email Routing API:
  https://developers.cloudflare.com/api/resources/email_routing/
- Routing Rules:
  https://developers.cloudflare.com/api/resources/email_routing/subresources/rules/
- Catch-all:
  https://developers.cloudflare.com/api/resources/email_routing/subresources/rules/subresources/catch_alls/
- Destination Addresses:
  https://developers.cloudflare.com/api/resources/email_routing/subresources/addresses/
- Email Routing configuration:
  https://developers.cloudflare.com/email-service/configuration/email-routing-addresses/
- Email Sending REST API:
  https://developers.cloudflare.com/api/resources/email_sending/
- Workers Scripts API:
  https://developers.cloudflare.com/api/resources/workers/subresources/scripts/

---

## 16. Outbound Email Sending Specification

Package: `internal/email/sending`

Endpoint:
```http
POST https://api.cloudflare.com/client/v4/accounts/{account_id}/email/sending/send
```

Request DTO:
```json
{
  "from": "sender@domain.com",
  "to": ["recipient@domain.com"],
  "subject": "Email Subject",
  "text": "Plain text body",
  "html": "<p>HTML body</p>",
  "headers": {
    "X-Custom-Header": "value"
  }
}
```

Response DTO:
```json
{
  "message_id": "cf_msg_12345",
  "status": "sent",
  "errors": []
}
```

Error Mapping:
- HTTP 400 / 10001, 10200-10202 &rarr; `domain.ErrCodeValidationFailed`
- HTTP 401/403 / 10101-10105 &rarr; `domain.ErrCodeUnauthorized`
- HTTP 404 / 10000 &rarr; `domain.ErrCodeNotFound`
- Destination unverified &rarr; `domain.ErrCodeDestinationNotVerified`
- HTTP 504 / timeout &rarr; `domain.ErrCodeProviderTimeout`
- HTTP 500/502/503 &rarr; `domain.ErrCodeProviderError`

---

## 17. Multi-User RBAC & OIDC Authentication Specification

Packages: `internal/auth/rbac`, `internal/auth/`

### Role Hierarchy & Permissions
- **Admin** (Level 3): Full system access (accounts, secrets, rules, destinations, sync, workers).
- **Operator** (Level 2): Rules, catch-all, destinations, sync, and workers modification. Cannot modify accounts or master secrets.
- **Viewer** (Level 1): Read-only access (`GET`, `HEAD`, `OPTIONS`) across resources.

### Middleware Enforcement
```go
rbac.RequireRole(rbac.RoleOperator)
rbac.RequirePermission(rbac.PermRulesModify)
```
On unauthorized access: Returns HTTP 403 Forbidden:
```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "Insufficient permissions"
  }
}
```

### OIDC Integration
- Parses `Authorization: Bearer <jwt>`
- Decodes claims: subject (`sub`), email (`email`), roles (`roles`, `realm_access.roles`)
- Validates signature (HMAC-SHA256) and expiration timestamp.
- Stores actor identity and role in request context.

---

## 18. Notification & Webhook Alerting Service Specification

Package: `internal/notification`

Configuration:
- `WebhookURL`: Target endpoint URL
- `WebhookType`: `slack`, `discord`, `telegram`, or `generic` (default: generic)
- `TelegramChatID`: Chat identifier for Telegram bots
- `Timeout`: HTTP timeout (default 10s)

Events:
1. **Drift Detected**: Dispatched when synchronization detects `CHANGED`, `REMOTE_ONLY`, or `LOCAL_ONLY` rules.
2. **Destination Verified**: Dispatched when a destination address confirmation is detected.

Payload Formats:
- **Slack**: Structured block kit with colored attachments.
- **Discord**: Embed payload with categorized fields for Changed, Remote, and Local rules.
- **Telegram**: Markdown-formatted message with chat_id.
- **Generic**: Standard JSON envelope with event type, timestamp, zone name, and diffs array.

---

## 19. Scheduled Background Reconciliation Worker Specification

Package: `internal/sync`

Worker Configuration:
```go
worker := sync.NewWorker(repos, syncEngine, interval, logger, onDriftFunc)
worker.Start(ctx)
```

Execution Behavior:
1. Runs initial drift detection pass immediately upon startup.
2. Ticks at configured interval (e.g. 5 minutes).
3. Queries active zones from repository.
4. Calls `SyncEngine.SyncZone(ctx, zone.ID, "drift_check", requestID)`.
5. If drift exists (`status != "MATCHED"`), triggers `onDriftFunc(zone, result)` callback.
6. Handles graceful cancellation via `ctx.Done()`.

---

## 20. Cloudflare Worker Script Deployment Specification

Package: `internal/provider/cloudflare`

API Methods:
```go
ListWorkers(ctx context.Context, accountID string) ([]WorkerSummary, error)
UploadWorker(ctx context.Context, accountID, scriptName, scriptContent string) error
DeleteWorker(ctx context.Context, accountID, scriptName string) error
```

Endpoints:
- `GET /accounts/{account_id}/workers/scripts`
- `PUT /accounts/{account_id}/workers/scripts/{script_name}` (`application/javascript`)
- `DELETE /accounts/{account_id}/workers/scripts/{script_name}`

---

## 21. Multi-Provider AWS SES Adapter Specification

Package: `internal/provider/ses`

Implements interface `provider.EmailProvider`:
- `ListZones`: Maps AWS SES domain identities (`ses:ListIdentities`, `IdentityType: Domain`).
- `GetEmailRoutingSettings` / `UpdateEmailRoutingSettings`: Maps active receipt rule set state (`ses:DescribeActiveReceiptRuleSet`).
- `ListDestinationAddresses` / `CreateDestinationAddress` / `DeleteDestinationAddress`: Maps SES verified email addresses (`IdentityType: EmailAddress`).
- `ListRules` / `CreateRule` / `UpdateRule` / `DeleteRule`: Maps SES receipt rules with recipient conditions and forwarding/drop actions.
- `GetCatchAll` / `UpdateCatchAll`: Maps catch-all receipt rule at end of rule set.

---

## 22. Web Dashboard & CLI Tool Specification

### Web Dashboard
- Single-page application embedded in binary via Go `embed` (`web/index.html`).
- Served at `GET /` and `GET /dashboard`.
- Communicates directly with `/api/v1` REST endpoints using session/API key.

### CLI Utility (`cmd/ems-cli`)
- Subcommands: `accounts`, `zones`, `destinations`, `rules`, `catch-all`, `sync`.
- Supports `--api-key` and `--url` flags or environment variables `EMS_API_KEY` and `EMS_API_URL`.
- Outputs human-readable tables or formatted JSON.

