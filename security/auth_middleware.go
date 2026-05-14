package security

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddlewareConfig holds configuration for authorization
type AuthMiddlewareConfig struct {
	ServiceKey    string   // Shared key for service-to-service communication
	ExcludedPaths []string // Paths that don't require authorization
}

// RequiredPermission specifies what permission is needed
type RequiredPermission string

const (
	PermissionPublic  RequiredPermission = "public"  // No auth required
	PermissionAdmin   RequiredPermission = "admin"   // Admin role required
	PermissionMeeting RequiredPermission = "meeting" // Admin or Manager with meeting access
)

var config *AuthMiddlewareConfig

// endpointPermissions maps endpoint paths to their required permissions
// Format: "METHOD /path" -> RequiredPermission
var endpointPermissions = make(map[string]RequiredPermission)

// InitAuthMiddleware initializes the auth middleware with configuration
func InitAuthMiddleware(cfg *AuthMiddlewareConfig) {
	config = cfg
}

// RegisterEndpoint registers an endpoint with its required permission level
// pathPattern: "/users", "/users/:id", "/meetings/:meet_id", etc.
// method: "GET", "POST", "PUT", "DELETE", "PATCH"
// permission: PermissionPublic, PermissionAdmin, or PermissionMeeting
func RegisterEndpoint(method string, pathPattern string, permission RequiredPermission) {
	key := fmt.Sprintf("%s %s", method, pathPattern)
	endpointPermissions[key] = permission
}

// RegisterEndpoints registers multiple endpoints at once
func RegisterEndpoints(endpoints map[string]map[string]RequiredPermission) {
	for method, paths := range endpoints {
		for path, permission := range paths {
			RegisterEndpoint(method, path, permission)
		}
	}
}

// AuthMiddleware is the Gin middleware that enforces authorization
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if config == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization middleware not initialized"})
			c.Abort()
			return
		}

		if isExcludedPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		method := c.Request.Method

		// OPTIONS requests are always allowed
		if method == http.MethodOptions {
			c.Next()
			return
		}

		// Check if this endpoint has explicit permission requirements
		permission := getEndpointPermission(method, c.Request.URL.Path)

		// PermissionPublic doesn't require auth or service key
		if permission == PermissionPublic {
			c.Next()
			return
		}

		// GET, HEAD requests without explicit PermissionAdmin/PermissionMeeting requirement are public
		if permission == "" && (method == http.MethodGet || method == http.MethodHead) {
			c.Next()
			return
		}

		// Check for service-to-service communication
		if isServiceRequest(c) {
			c.Next()
			return
		}

		// For data-modifying requests or admin endpoints, check token
		claims, err := extractAndValidateToken(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		// Store claims in context for use by handlers
		c.Set("keycloak_claims", claims)
		c.Set("user_id", claims.Subject)

		// Check authorization based on endpoint permission
		switch permission {
		case PermissionAdmin:
			if !claims.HasRole("admin") {
				c.JSON(http.StatusForbidden, gin.H{"error": "admin role required for this operation"})
				c.Abort()
				return
			}

		case PermissionMeeting:
			meetingID := extractMeetingIDFromRequest(c)
			if !isAuthorizedForMeeting(claims, meetingID) {
				c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this meeting"})
				c.Abort()
				return
			}

		default:
			// Backward compatibility: if no explicit permission and not a safe method,
			// check for meeting-specific endpoint
			meetingID := extractMeetingIDFromRequest(c)
			if meetingID != "" {
				if !isAuthorizedForMeeting(claims, meetingID) {
					c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this meeting"})
					c.Abort()
					return
				}
			} else {
				// General endpoint - requires admin role
				if !claims.HasRole("admin") {
					c.JSON(http.StatusForbidden, gin.H{"error": "admin role required for this operation"})
					c.Abort()
					return
				}
			}
		}

		c.Next()
	}
}

func isExcludedPath(path string) bool {
	for _, excludedPath := range config.ExcludedPaths {
		if excludedPath == "" {
			continue
		}

		if path == excludedPath || strings.HasPrefix(path, excludedPath+"/") {
			return true
		}
	}

	return false
}

// getEndpointPermission looks up the required permission for an endpoint
// Returns empty string if no explicit permission is registered
func getEndpointPermission(method string, path string) RequiredPermission {
	// Try exact match first
	key := fmt.Sprintf("%s %s", method, path)
	if permission, exists := endpointPermissions[key]; exists {
		return permission
	}

	// Try to match path patterns (for routes with parameters like /users/:id)
	// This is a simple prefix match - in production, use proper route matching
	for registeredKey, permission := range endpointPermissions {
		parts := strings.Split(registeredKey, " ")
		if len(parts) != 2 {
			continue
		}
		registeredMethod, registeredPath := parts[0], parts[1]

		if registeredMethod != method {
			continue
		}

		// Check if path matches pattern (simple version - checks if pattern matches)
		if pathMatches(registeredPath, path) {
			return permission
		}
	}

	return ""
}

// pathMatches checks if a pattern like "/users/:id" matches a path like "/users/123"
func pathMatches(pattern string, path string) bool {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) != len(pathParts) {
		return false
	}

	for i, patPart := range patternParts {
		// Parameters like :id or :meet_id match anything
		if strings.HasPrefix(patPart, ":") {
			continue
		}
		// Literal parts must match exactly
		if patPart != pathParts[i] {
			return false
		}
	}

	return true
}

// isServiceRequest checks if the request is from another service using SR_SERVICE_KEY
func isServiceRequest(c *gin.Context) bool {
	if config.ServiceKey == "" {
		return false
	}

	serviceKey := c.GetHeader("X-Swimresults-Service")
	return serviceKey == config.ServiceKey
}

// extractAndValidateToken extracts JWT from Authorization header and validates it
func extractAndValidateToken(c *gin.Context) (*KeycloakClaims, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("missing authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	tokenString := parts[1]

	// Parse without verification (API Gateway handles verification)
	// In production, you might want to verify the token signature
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &KeycloakClaims{})
	if err != nil {
		return nil, fmt.Errorf("invalid token format: %v", err)
	}

	claims, ok := token.Claims.(*KeycloakClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// isAuthorizedForMeeting checks if user has admin role or manager role with meeting access
func isAuthorizedForMeeting(claims *KeycloakClaims, meetingID string) bool {
	// Admin can do everything
	if claims.HasRole("admin") {
		return true
	}

	// Manager needs matching event in claims
	if claims.HasRole("manager") && claims.HasEvent(meetingID) {
		return true
	}

	return false
}

// extractMeetingIDFromRequest tries to find meeting ID in the request
// This is a helper that works with common patterns
func extractMeetingIDFromRequest(c *gin.Context) string {
	// Check URL parameters first (most common)
	if meetID := c.Param("meet_id"); meetID != "" {
		return meetID
	}
	if meetID := c.Param("meetid"); meetID != "" {
		return meetID
	}
	if meetID := c.Param("meeting"); meetID != "" {
		return meetID
	}

	// Check query parameters
	if meetID := c.Query("meet_id"); meetID != "" {
		return meetID
	}
	if meetID := c.Query("meetid"); meetID != "" {
		return meetID
	}
	if meetID := c.Query("meeting"); meetID != "" {
		return meetID
	}

	// Check JSON body for meeting-related fields
	var body map[string]interface{}
	if err := c.BindJSON(&body); err == nil {
		if meetID, ok := body["meet_id"].(string); ok && meetID != "" {
			return meetID
		}
		if meetID, ok := body["meetid"].(string); ok && meetID != "" {
			return meetID
		}
		if meetID, ok := body["meeting"].(string); ok && meetID != "" {
			return meetID
		}
	}

	return ""
}

// GetKeycloakClaims retrieves claims from context
func GetKeycloakClaims(c *gin.Context) *KeycloakClaims {
	if claims, exists := c.Get("keycloak_claims"); exists {
		if claimsObj, ok := claims.(*KeycloakClaims); ok {
			return claimsObj
		}
	}
	return nil
}

// GetUserID retrieves the user ID from context
func GetUserID(c *gin.Context) string {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	return ""
}
