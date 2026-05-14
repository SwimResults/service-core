package security

import (
	"github.com/golang-jwt/jwt/v5"
)

// KeycloakClaims represents the structure of a Keycloak JWT token
type KeycloakClaims struct {
	jwt.RegisteredClaims
	RealmAccess struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	Events []string `json:"events"`
}

// HasRole checks if the token has a specific realm role
func (k *KeycloakClaims) HasRole(role string) bool {
	for _, r := range k.RealmAccess.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasEvent checks if a specific event/meeting ID is in the events list
func (k *KeycloakClaims) HasEvent(eventID string) bool {
	for _, e := range k.Events {
		if e == eventID {
			return true
		}
	}
	return false
}
