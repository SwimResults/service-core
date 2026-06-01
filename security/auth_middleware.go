package security

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddlewareConfig holds configuration for authorization
type AuthMiddlewareConfig struct {
	ServiceKey     string   // Shared key for service-to-service communication
	ExcludedPaths  []string // Paths that don't require authorization
	TokenPublicKey string   // Keycloak public key in PEM or base64-encoded PKIX format
}

// RequiredPermission specifies what permission is needed
type RequiredPermission string

const (
	PermissionPublic  RequiredPermission = "public"  // No auth required
	PermissionAdmin   RequiredPermission = "admin"   // Admin role required
	PermissionManager RequiredPermission = "manager" // Manager role meeting independent access
	PermissionMeeting RequiredPermission = "meeting" // Admin or Manager with meeting access
)

var config *AuthMiddlewareConfig

// endpointPermissions maps endpoint paths to their required permissions
// Format: "METHOD /path" -> RequiredPermission
var endpointPermissions = make(map[string]RequiredPermission)

// InitAuthMiddleware initializes the auth middleware with configuration
func InitAuthMiddleware(cfg *AuthMiddlewareConfig) {
	if cfg != nil && cfg.TokenPublicKey == "" {
		cfg.TokenPublicKey = strings.TrimSpace(os.Getenv("SR_JWT_PUBLIC_KEY"))
		if cfg.TokenPublicKey == "" {
			cfg.TokenPublicKey = strings.TrimSpace(os.Getenv("SR_KEYCLOAK_PUBLIC_KEY"))
		}
	}
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

// Route registers a Gin route and binds the permission to the same path definition.
// This keeps the path string in one place and avoids a separate permission registry.
func Route(router gin.IRoutes, method string, path string, permission RequiredPermission, handlers ...gin.HandlerFunc) gin.IRoutes {
	RegisterEndpoint(method, path, permission)
	return router.Handle(method, path, handlers...)
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

		case PermissionManager:
			if !isAuthorizedForManager(claims) {
				c.JSON(http.StatusForbidden, gin.H{"error": "manager role required for this operation"})
				c.Abort()
				return
			}

		case PermissionMeeting:
			meetingID, err := extractMeetingIDFromRequest(c)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
			if !isAuthorizedForMeeting(claims, meetingID) {
				c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this meeting"})
				c.Abort()
				return
			}

		default:
			// Backward compatibility: if no explicit permission and not a safe method,
			// check for meeting-specific endpoint
			meetingID, err := extractMeetingIDFromRequest(c)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
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
	return ValidateAuthorizationHeader(authHeader)
}

// ValidateAuthorizationHeader parses and validates a bearer token using the configured public key.
func ValidateAuthorizationHeader(authHeader string) (*KeycloakClaims, error) {
	if authHeader == "" {
		return nil, fmt.Errorf("missing authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	publicKey, err := resolveTokenPublicKey()
	if err != nil {
		return nil, err
	}

	claims := &KeycloakClaims{}
	token, err := jwt.ParseWithClaims(
		parts[1],
		claims,
		func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
			}
			return publicKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %v", err)
	}
	if token == nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

func resolveTokenPublicKey() (*rsa.PublicKey, error) {
	if config == nil {
		return nil, fmt.Errorf("authorization middleware not initialized")
	}

	keyMaterial := strings.TrimSpace(config.TokenPublicKey)
	if keyMaterial == "" {
		return nil, fmt.Errorf("token verification key not configured")
	}

	if block, _ := pem.Decode([]byte(keyMaterial)); block != nil {
		parsedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("invalid token public key: %v", err)
		}
		publicKey, ok := parsedKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("token public key is not rsa")
		}
		return publicKey, nil
	}

	decodedKey, err := base64.StdEncoding.DecodeString(keyMaterial)
	if err != nil {
		return nil, fmt.Errorf("invalid token public key encoding: %v", err)
	}

	parsedKey, err := x509.ParsePKIXPublicKey(decodedKey)
	if err != nil {
		return nil, fmt.Errorf("invalid token public key: %v", err)
	}
	publicKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("token public key is not rsa")
	}

	return publicKey, nil
}

// isAuthorizedForManager checks if user has admin role or manager role
func isAuthorizedForManager(claims *KeycloakClaims) bool {
	// Admin can do everything
	if claims.HasRole("admin") {
		return true
	}

	// Manager can do everything
	if claims.HasRole("manager") {
		return true
	}

	return false
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

// extractMeetingIDFromRequest tries to find a meeting ID without letting one source override another.
func extractMeetingIDFromRequest(c *gin.Context) (string, error) {
	var candidates []string

	addCandidate := func(value string) {
		if value != "" {
			candidates = append(candidates, value)
		}
	}

	// URL parameters are the strongest signal.
	addCandidate(c.Param("meet_id"))
	addCandidate(c.Param("meetid"))
	addCandidate(c.Param("meeting"))

	// Query parameters are still supported, but they must agree with other sources.
	addCandidate(c.Query("meet_id"))
	addCandidate(c.Query("meetid"))
	addCandidate(c.Query("meeting"))

	// Check JSON body for meeting-related fields without consuming it for downstream handlers.
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read request body: %v", err)
	}
	if len(rawBody) > 0 {
		c.Request.Body = io.NopCloser(bytes.NewBuffer(rawBody))

		var body map[string]interface{}
		if err := json.Unmarshal(rawBody, &body); err == nil {
			if meetID, ok := body["meet_id"].(string); ok && meetID != "" {
				addCandidate(meetID)
			}
			if meetID, ok := body["meetid"].(string); ok && meetID != "" {
				addCandidate(meetID)
			}
			if meetID, ok := body["meeting"].(string); ok && meetID != "" {
				addCandidate(meetID)
			}
		}
	}

	if len(candidates) == 0 {
		return "", nil
	}

	meetingID := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate != meetingID {
			return "", fmt.Errorf("conflicting meeting identifiers in request")
		}
	}

	return meetingID, nil
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
