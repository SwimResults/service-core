# Service Core

This module contains the shared building blocks used by the swimresults microservices. The main piece in this workspace is the Gin authorization middleware that centralizes Keycloak-based access control for the API-facing services.

## What It Does

The middleware in [security/auth_middleware.go](security/auth_middleware.go) handles three concerns:

1. Public request handling for safe methods.
2. JWT-based authorization for write operations.
3. Service-to-service access via `SR_SERVICE_KEY`.

The claims helper in [security/keycloak_claims.go](security/keycloak_claims.go) models the Keycloak token shape used by the services.

## Authorization Model

The rules are intentionally simple:

- `GET`, `HEAD`, and `OPTIONS` are allowed without a token.
- `POST`, `PUT`, `PATCH`, `DELETE`, and other write methods require authorization.
- General write endpoints require the realm role `admin`.
- Meeting-specific write endpoints allow either `admin` or `manager` when the token also contains the meeting ID in the `events` claim.
- Requests with `X-Swimresults-Service: <SR_SERVICE_KEY>` bypass token checks and are treated as trusted inter-service calls.

## Supported Token Shape

The middleware expects a Keycloak token shaped like this:

```json
{
	"realm_access": {
		"roles": ["private", "manager", "admin"]
	},
	"events": ["BGF26", "BMLSSWS26"],
	"sub": "user-id"
}
```

Only the fields used by authorization are required:

- `realm_access.roles` for role checks.
- `events` for meeting-specific access.
- `sub` is stored in Gin context for handlers that need the caller identity.

## Service Behavior

### start-service

Start-service uses the middleware for all routes. Read-only endpoints remain public. Write endpoints are admin-only unless they clearly identify a meeting and the caller has manager access for that meeting.

### athlete-service

Athlete-service follows the same rule set as start-service. Public reads stay open, write routes are protected, and service-to-service calls can use the shared key.

### meeting-service

Meeting-service also uses the shared middleware. Meeting-specific routes are the main place where the manager-plus-events rule applies.

### import-service

Import-service uses the same authorization model, but the `/easywk` endpoints are excluded because they keep their own password-based authentication flow. Those routes stay untouched.

### user-service

User-service already has its own permission model around user identity and personal data. It was left on the existing auth path instead of being forced into the new middleware model.

## Middleware Flow

The request flow is straightforward:

1. Skip excluded paths such as `/actuator` and `/easywk`.
2. Allow safe methods without token checks.
3. Allow requests with the shared service header.
4. Parse the bearer token.
5. Extract the caller claims.
6. Detect whether the request is meeting-specific.
7. Allow `admin` for all protected writes.
8. Allow `manager` only when the meeting matches an entry in `events`.
9. Reject everything else with `401` or `403`.

The exclusion handling now lives in the middleware itself, so controller setup stays declarative and does not need duplicated bypass logic.

## Configuration

The services only need one shared value for inter-service calls:

```bash
SR_SERVICE_KEY=shared-secret-value
```

Each service passes that value into the middleware during startup.

Example setup:

```go
security.InitAuthMiddleware(&security.AuthMiddlewareConfig{
	ServiceKey:      os.Getenv("SR_SERVICE_KEY"),
	ExcludedPaths:   []string{"/actuator"},
	MeetingIDFields: make(map[string]string),
})

router.Use(security.AuthMiddleware())
```

For import-service, the excluded paths include both `/actuator` and `/easywk`.

## Meeting IDs

The middleware currently looks for meeting identifiers in the common names already used across the codebase:

- `meet_id`
- `meetid`
- `meeting`

It checks URL params, query params, and JSON bodies in that order. If a route uses a different name, that name can be added in one place inside the middleware.

## Adding a New Protected Route

When adding a new write endpoint:

1. Decide whether it is general or meeting-specific.
2. If it is meeting-specific, make sure the request exposes the meeting identifier under one of the supported names.
3. Keep the controller routing unchanged; the middleware will enforce the access rule before the handler runs.

## Example Access Rules

### Public read

`GET /start/meet/BGF26` is allowed without a token.

### General write

`POST /ranking` requires the `admin` role unless the request comes from another service using `SR_SERVICE_KEY`.

### Meeting-specific write

`POST /start/meet/BGF26/event/123` is allowed for:

- `admin`
- `manager` with `BGF26` in `events`
- trusted service-to-service requests

## Testing

The quickest checks are:

```bash
curl http://localhost:<port>/actuator
curl http://localhost:<port>/start
curl -X POST http://localhost:<port>/start -H "Authorization: Bearer <token>"
curl -X POST http://localhost:<port>/start -H "X-Swimresults-Service: <key>"
```

Expected behavior:

- Public reads succeed without a token.
- Protected writes fail without authorization.
- Admin tokens succeed everywhere.
- Manager tokens only succeed when the meeting matches.

## Notes

- The middleware assumes API gateway validation is already in front of the services.
- The user-service stays on its existing auth model because it needs user-identity-specific behavior.
- `/easywk` in import-service is intentionally excluded so the legacy password flow remains intact.

## Files In This Module

- [security/auth_middleware.go](security/auth_middleware.go)
- [security/keycloak_claims.go](security/keycloak_claims.go)

This README is the single documentation entry point for the shared authorization work in service-core.