# User-Service Authorization Middleware Integration Plan

## Overview
User-service can adopt the new `RequiredPermission` decorator pattern while preserving its current identity-verification logic. The service already parses tokens and checks service keys—we can layer the centralized permission system on top.

## Current User-Service Architecture

### Authentication Flow
```
checkIfRoot() 
├─ checkServiceKey() [checks X-Swimresults-Service header]
└─ checkAuthHeaderToken() [checks Authorization bearer token, verifies admin role]
```

### Endpoint Categories
1. **Public (no auth)**: `/notification_user/public/*`, `/notification_user/public/register`
   - Currently work without any auth checks
   - Use custom tokens (registration tokens, device tokens)

2. **Admin-only**: `/notification_users`, `/notification_user/:id`, `/user`, `/users`, etc.
   - Call `failIfNotRoot()` which requires service key OR admin role
   - Respond with 401 if missing either

3. **User-specific (authenticated)**: `/notification_user`, `/user`, `/user/me`, etc.
   - Extract claims from Authorization header
   - Look up user by keycloak ID from claims
   - Return that user's own data (identity check in handler)
   - Currently fail silently if no token

## Integration Strategy

### Phase 1: Add Permission Registrations (No Code Changes Required)
In `controller.go`, after calling middleware functions, register endpoint permissions:

```go
func init() {
    // Public endpoints - accessible without token
    security.RegisterEndpoints(map[string]map[string]security.RequiredPermission{
        "GET": {
            "/notification_user/public/token/:token":     security.PermissionPublic,
            "/notification_user/public/register":         security.PermissionPublic,
        },
        "POST": {
            "/notification_user/public/register":         security.PermissionPublic,
        },
        // Admin-only endpoints
        "GET": {
            "/notification_users":  security.PermissionAdmin,
            "/notification_user/:id": security.PermissionAdmin,
            "/users":               security.PermissionAdmin,
            "/user/:id":            security.PermissionAdmin,
            // ...more admin endpoints
        },
        "POST": {
            "/notification_user":          security.PermissionAdmin,
            "/notification_user/:id":      security.PermissionAdmin,
            "/user":                       security.PermissionAdmin,
            "/dashboard":                  security.PermissionAdmin,
            "/config":                     security.PermissionAdmin,
            // ...more admin endpoints
        },
        "PUT": {
            "/notification_user": security.PermissionAdmin,
            "/user":              security.PermissionAdmin,
        },
        "DELETE": {
            "/notification_user/:id": security.PermissionAdmin,
            "/user/:id":              security.PermissionAdmin,
            "/dashboard/:id":         security.PermissionAdmin,
            "/report/:id":            security.PermissionAdmin,
        },
    })
}
```

### Phase 2: Activate Middleware (Optional - for Centralized Enforcement)
If you want centralized permission enforcement:

```go
func Run() {
    // ... existing setup ...
    
    serviceKey = os.Getenv("SR_SERVICE_KEY")
    if serviceKey == "" {
        fmt.Println("no security for inter-service communication given! Please set SR_SERVICE_KEY.")
        return
    }
    
    // Register permissions BEFORE starting router
    registerEndpointPermissions()
    
    // Initialize and attach middleware
    security.InitAuthMiddleware(&security.AuthMiddlewareConfig{
        ServiceKey:    serviceKey,
        ExcludedPaths: []string{"/actuator"},
    })
    router.Use(security.AuthMiddleware())
    
    // ... rest of setup ...
}
```

### Phase 3: Preserve Identity Checks in Handlers
**No changes needed in handler logic!**

Handlers that need user identity verification keep their current pattern:
```go
func getNotificationUser(c *gin.Context) {
    // Middleware already validated token for PermissionPublic endpoints
    // Handler can extract claims and verify ownership
    claims, err := getClaimsFromAuthHeader(c)
    if err != nil {
        c.IndentedJSON(http.StatusNotFound, gin.H{"message": err.Error()})
        return
    }
    
    // Identity check: ensure they're looking up their own data
    user, err := service.GetUserByKeycloakId(claims.Sub)
    if err != nil {
        c.IndentedJSON(http.StatusNotFound, gin.H{"message": err.Error()})
        return
    }
    
    notificationUser, err := service.GetNotificationUserByUserId(user.Identifier)
    // ... rest of logic
}
```

## Behavior Comparison

### Current (Path-Based)
- Public endpoints: Named with `/public/` in path
- Admin endpoints: Implicit, call `failIfNotRoot()`
- User endpoints: Extract claims in handler, no middleware enforcement

### New (Permission-Explicit)
- Public endpoints: Registered as `PermissionPublic`, accessible without token
- Admin endpoints: Registered as `PermissionAdmin`, middleware enforces
- User endpoints: Still use `PermissionPublic` but handler does identity verification

## Key Advantages

1. **Single Source of Truth**: All endpoint permissions in one place (or in init block)
2. **Backward Compatible**: Service key check still works, handlers keep identity logic
3. **Fail-Safe**: Unregistered endpoints default to safe HTTP methods (GET/HEAD) without token, fail-safe on POST/PUT/DELETE
4. **Explicit Intent**: Clear what each endpoint requires just by reading registration code
5. **Unified Security**: Uses same permission framework as other 4 services

## Example Implementation

### Before (Current)
```go
// In notification_user_controller.go
func getNotificationUser(c *gin.Context) {
    claims, err := getClaimsFromAuthHeader(c)
    if err != nil {
        c.IndentedJSON(http.StatusNotFound, gin.H{"message": err.Error()})
        return
    }
    // ... use claims
}

func getNotificationUsers(c *gin.Context) {
    if failIfNotRoot(c) {
        return
    }
    // ... get all users
}
```

### After (With Middleware)
```go
// In controller.go init()
security.RegisterEndpoints(map[string]map[string]security.RequiredPermission{
    "GET": {
        "/notification_user":         security.PermissionPublic,  // Identity check in handler
        "/notification_users":        security.PermissionAdmin,   // Enforced by middleware
    },
})

// Handler code stays the same!
func getNotificationUser(c *gin.Context) {
    claims := security.GetKeycloakClaims(c)  // Already extracted by middleware
    if claims == nil {
        c.IndentedJSON(http.StatusNotFound, gin.H{"message": "no claims"})
        return
    }
    // ... use claims (guaranteed valid by middleware)
}

func getNotificationUsers(c *gin.Context) {
    // No failIfNotRoot() call needed - middleware already enforced admin role
    users, err := service.GetNotificationUsers()
    // ... rest of logic
}
```

## Migration Path (Optional Phases)

### Phase 1 (Recommended): Register but Don't Activate Middleware
- Register endpoint permissions
- Middleware stays inactive (for documentation)
- Keep existing `failIfNotRoot()` calls
- No behavior changes, just prep work

### Phase 2: Activate Middleware Incrementally
- Enable middleware
- Update admin endpoints to remove `failIfNotRoot()` calls
- Let middleware enforce permissions
- Keep handler identity checks intact

### Phase 3 (Optional): Unify with Other Services
- Use same middleware as start-service, athlete-service, etc.
- Consistent authorization model across platform

## Endpoint Permission Mapping

### Public Endpoints (PermissionPublic)
```
GET  /notification_user/public/token/:token
POST /notification_user/public/register
GET  /notification_user/public/register
POST /notification_user/register
```

### Authenticated User Endpoints (PermissionPublic with handler identity check)
```
GET  /notification_user      (handler checks claims.Sub matches user)
POST /user/me                (handler uses claims.Sub)
POST /user/language
POST /user/theme
```

### Admin-Only Endpoints (PermissionAdmin)
```
GET    /notification_users
GET    /notification_user/:id
GET    /users
GET    /user/:id
GET    /config
GET    /widget
GET    /dashboard
GET    /report
POST   /notification_user
POST   /user
POST   /user/athlete
POST   /user/meeting
POST   /config
POST   /dashboard
POST   /report
PUT    /notification_user
PUT    /user
DELETE /notification_user/:id
DELETE /user/:id
DELETE /dashboard/:id
DELETE /report/:id
```

## Recommendations

1. **Start with Phase 1**: Register permissions without activating middleware—this documents intent with zero behavior changes
2. **Test in dev**: Activate middleware in development to verify permission logic
3. **Gradual rollout**: Update handlers to use `security.GetKeycloakClaims()` instead of manually extracting
4. **Remove duplication**: Once middleware is active, remove `failIfNotRoot()` calls from admin endpoints
5. **Keep identity checks**: Never remove the handler-level identity verification for user-specific endpoints
