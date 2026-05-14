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

// InitAuthMiddleware initializes the auth middleware with configuration
func InitAuthMiddleware(cfg *AuthMiddlewareConfig) {
	config = cfg
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

		// GET, HEAD requests are allowed for everyone without token
		if method == http.MethodGet || method == http.MethodHead {
			c.Next()
			return
		}

		// Check for service-to-service communication
		if isServiceRequest(c) {
			c.Next()
			return
		}

		// For data-modifying requests, check token
		claims, err := extractAndValidateToken(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		// Store claims in context for use by handlers
		c.Set("keycloak_claims", claims)
		c.Set("user_id", claims.Subject)

		// Check authorization based on endpoint type
		meetingID := extractMeetingIDFromRequest(c)
		if meetingID != "" {
			// This is a meeting-specific endpoint
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
