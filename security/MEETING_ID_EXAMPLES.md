# Authorization Middleware - Meeting ID Extraction Examples

This document shows examples of how the middleware automatically detects meeting IDs in different endpoint patterns and enforces authorization.

## Start Service Examples

### Example 1: URL Parameter (Automatic)
**Endpoint:** `DELETE /start/meet/:meet_id/event/:event_id`

The middleware automatically extracts `meet_id` from the URL path.

```
DELETE /start/meet/BGF26/event/123

Authorization check:
- User has admin role? ✓ Allowed
- User has manager role AND has "BGF26" in events claim? ✓ Allowed
- User has manager role but NO "BGF26" in events? ✗ Forbidden
```

### Example 2: JSON Body Parameter (Automatic)
**Endpoint:** `POST /start`

The model requires meeting information in the JSON body:
```json
{
  "meeting": "BGF26",
  "event": 123,
  "heat_number": 1,
  "lane": 1,
  "athlete": "...",
  ...
}
```

The middleware extracts `meeting` from the JSON body:
```
POST /start
{
  "meeting": "BGF26",
  ...
}

Authorization check: Same as above
```

### Example 3: Import Endpoints (Admin Required)
**Endpoints:** 
- `POST /start/import`
- `POST /heat/import`
- `POST /ranking/import`

These endpoints typically process bulk data without a specific meeting context in the request, so they require **admin** role.

## Meeting Service Examples

### Example 1: Meeting Resource Operations
**Endpoint:** `POST /meeting/:id` (update meeting)

Even though the URL parameter is `:id` (not `:meet_id`), if the request body or endpoint logic is about meetings, the developer should ensure the meeting ID is extractable. 

**Option A:** Update the middleware to recognize the pattern:
```go
// In security/auth_middleware.go, add:
if meetID := c.Param("id"); c.Request.URL.Path contains "meeting" {
    // treat as meeting
}
```

**Option B:** Include meeting ID in request body:
```json
POST /meeting/65a3f2e8c1234567890abc
{
  "meeting_id": "BGF26",
  "name": "Updated Name",
  ...
}
```

### Example 2: Event Management (Meeting-Specific)
**Endpoint:** `POST /event` (create/update event for a meeting)

If events must be associated with a meeting, include the meeting ID:
```json
POST /event
{
  "meeting": "BGF26",  // or "meet_id"
  "number": 1,
  "name": "100m Freestyle",
  ...
}
```

## Import Service - Special Handling

### EasyWk Endpoints (Password Protected)
**Endpoints:** `/easywk/*`

These endpoints are **completely excluded** from the new authorization middleware and continue to use password authentication.

```
POST /easywk/livework.php
POST /easywk/v2/livework.php
POST /easywk/v3

No JWT token required
Handled by legacy password authentication system
```

## How Meeting ID Detection Works

The middleware checks in this order:

1. **URL Parameters:**
   - `c.Param("meet_id")`
   - `c.Param("meetid")`
   - `c.Param("meeting")`

2. **Query Parameters:**
   - `c.Query("meet_id")`
   - `c.Query("meetid")`
   - `c.Query("meeting")`

3. **JSON Body:**
   - `body["meet_id"]`
   - `body["meetid"]`
   - `body["meeting"]`

## Adding Support for Custom Parameter Names

If your endpoint uses a different field name for the meeting ID, you have two options:

### Option 1: Update the Middleware (One-time effort)

Edit `service-core/security/auth_middleware.go` and add your custom parameter names to `extractMeetingIDFromRequest()`:

```go
func extractMeetingIDFromRequest(c *gin.Context) string {
    // Check URL parameters first
    if meetID := c.Param("meet_id"); meetID != "" {
        return meetID
    }
    if meetID := c.Param("meetid"); meetID != "" {
        return meetID
    }
    if meetID := c.Param("meeting"); meetID != "" {
        return meetID
    }
    
    // ADD YOUR CUSTOM NAMES HERE:
    if meetID := c.Param("meeting_id"); meetID != "" {  // Snake case variant
        return meetID
    }
    if meetID := c.Param("comp_id"); meetID != "" {     // Competition ID
        return meetID
    }
    
    // ... rest of the function
}
```

### Option 2: Normalize Data in Your Service

Make sure your request models always map to `meeting`, `meet_id`, or `meetid`:

```go
type UpdateMeetingRequest struct {
    MeetingID string `json:"competition_id" binding:"required"`  // Custom JSON field
    // ...
}

func (r *UpdateMeetingRequest) GetMeetingID() string {
    return r.MeetingID  // Normalized access
}
```

Then in your handler, you can manually extract and pass it through:
```go
var req UpdateMeetingRequest
c.BindJSON(&req)
// The middleware will check the JSON body for "competition_id"
```

## Token Examples

### Admin User
```json
{
  "realm_access": {
    "roles": ["admin", "offline_access"]
  },
  "events": [],
  "sub": "user-123"
}
```
**Permissions:** Can perform any operation on any meeting.

### Manager User (Multiple Meetings)
```json
{
  "realm_access": {
    "roles": ["manager", "offline_access"]
  },
  "events": ["BGF26", "BMLSSWS26", "BMSWS26"],
  "sub": "user-456"
}
```
**Permissions:** 
- ✓ GET requests (any data)
- ✓ POST/PUT/DELETE for meetings: BGF26, BMLSSWS26, BMSWS26
- ✗ POST/PUT/DELETE for other meetings

### Manager User (No Specific Meeting Access)
```json
{
  "realm_access": {
    "roles": ["manager", "offline_access"]
  },
  "events": [],
  "sub": "user-789"
}
```
**Permissions:**
- ✓ GET requests (any data)
- ✗ POST/PUT/DELETE (insufficient permissions)

## Testing Your Endpoints

### Test 1: Verify GET is Always Public
```bash
# No token needed
curl -X GET http://localhost:8080/start
# Should return 200 OK
```

### Test 2: Verify POST Needs Authorization
```bash
# Without token
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -d '{"meeting":"BGF26","event":1,"lane":1}'
# Should return 401 Unauthorized

# With admin token
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <admin_token>" \
  -d '{"meeting":"BGF26","event":1,"lane":1}'
# Should return 200 OK (or actual response)
```

### Test 3: Verify Manager with Meeting Access
```bash
# Manager token with "BGF26" in events claim
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <manager_token_with_BGF26>" \
  -d '{"meeting":"BGF26","event":1,"lane":1}'
# Should return 200 OK

# Same token, different meeting
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <manager_token_with_BGF26>" \
  -d '{"meeting":"OTHER26","event":1,"lane":1}'
# Should return 403 Forbidden
```

### Test 4: Service-to-Service Communication
```bash
# Using service key instead of user token
curl -X POST http://localhost:8080/start \
  -H "Content-Type: application/json" \
  -H "X-Swimresults-Service: your_service_key_here" \
  -d '{"meeting":"BGF26","event":1,"lane":1}'
# Should return 200 OK (no user auth required)
```

## Common Issues and Solutions

### Issue: "insufficient permissions for this meeting"
**Cause:** User has manager role but the meeting ID in the request doesn't match their events claim.

**Solution:** Check that:
1. The meeting ID is being extracted correctly (add logging to see what's extracted)
2. The Keycloak events claim includes the meeting ID
3. The request includes the meeting ID in URL, query, or body

### Issue: "admin role required for this operation"
**Cause:** No meeting ID was found in the request, so it requires admin.

**Solution:**
1. Add the meeting ID to the request (URL path, query param, or JSON body)
2. OR grant the user admin role
3. OR check that the middleware is correctly extracting the meeting ID

### Issue: Endpoints are unreachable
**Cause:** Middleware is blocking legitimate requests.

**Solution:**
1. Check if endpoint should be excluded (like /actuator, /easywk)
2. Check if GET request is being incorrectly blocked (GET should always be public)
3. Enable debug logging in middleware to see what's being extracted

