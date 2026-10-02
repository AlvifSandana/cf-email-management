package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bariskode/email-management-service/internal/auth/rbac"
	"github.com/bariskode/email-management-service/internal/domain"
)

const (
	ActorRoleKey  contextKey = "actor_role"
	ActorEmailKey contextKey = "actor_email"
)

// Claims holds standard OIDC JWT claims along with role assignments.
type Claims struct {
	Subject       string         `json:"sub"`
	Email         string         `json:"email"`
	EmailVerified bool           `json:"email_verified,omitempty"`
	Roles         []string       `json:"roles,omitempty"`
	Role          string         `json:"role,omitempty"`
	Issuer        string         `json:"iss,omitempty"`
	Audience      any            `json:"aud,omitempty"`
	ExpiresAt     int64          `json:"exp,omitempty"`
	NotBefore     int64          `json:"nbf,omitempty"`
	IssuedAt      int64          `json:"iat,omitempty"`
	Raw           map[string]any `json:"-"`
}

// UnmarshalJSON provides flexible decoding for OIDC claims across various providers.
func (c *Claims) UnmarshalJSON(data []byte) error {
	type Alias Claims
	aux := &struct {
		RolesAny any `json:"roles"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Capture all raw claims for flexible extraction
	_ = json.Unmarshal(data, &c.Raw)

	// Normalize roles from various JSON formats:
	// 1. Array of strings or interfaces: ["operator", "admin"]
	// 2. Comma-separated string: "operator,admin"
	switch v := aux.RolesAny.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				c.Roles = append(c.Roles, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, item := range v {
			if strings.TrimSpace(item) != "" {
				c.Roles = append(c.Roles, strings.TrimSpace(item))
			}
		}
	case string:
		for _, part := range strings.Split(v, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				c.Roles = append(c.Roles, trimmed)
			}
		}
	}

	// Single "role" claim fallback
	if len(c.Roles) == 0 && strings.TrimSpace(c.Role) != "" {
		c.Roles = append(c.Roles, strings.TrimSpace(c.Role))
	}

	// Keycloak realm_access.roles fallback
	if len(c.Roles) == 0 && c.Raw != nil {
		if realmAccess, ok := c.Raw["realm_access"].(map[string]any); ok {
			if rList, ok := realmAccess["roles"].([]any); ok {
				for _, item := range rList {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						c.Roles = append(c.Roles, strings.TrimSpace(s))
					}
				}
			}
		}
	}

	return nil
}

// PrimaryRole resolves the highest-privilege role present in the claims.
// Priority: Admin > Operator > Viewer.
func (c *Claims) PrimaryRole() rbac.Role {
	var highest rbac.Role
	highestLevel := 0

	candidates := make([]string, 0, len(c.Roles)+1)
	candidates = append(candidates, c.Roles...)
	if c.Role != "" {
		candidates = append(candidates, c.Role)
	}

	for _, cand := range candidates {
		r := rbac.Role(strings.ToLower(strings.TrimSpace(cand)))
		level := r.Level()
		if level > highestLevel {
			highest = r
			highestLevel = level
		}
	}

	return highest
}

// ExtractBearerToken parses the Bearer token from the Authorization header.
func ExtractBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("missing Authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid Authorization header format, expected 'Bearer <token>'")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("empty bearer token")
	}

	return token, nil
}

// decodeBase64URL decodes raw or standard URL-safe base64 strings.
func decodeBase64URL(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

// ParseJWT parses an unverified or verified JWT string and decodes its Claims.
func ParseJWT(tokenString string) (*Claims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT token structure, expected 3 parts")
	}

	payloadBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims JSON: %w", err)
	}

	// Verify expiration time
	if claims.ExpiresAt > 0 {
		now := time.Now().Unix()
		if now > claims.ExpiresAt {
			return nil, errors.New("token is expired")
		}
	}

	// Verify not-before time
	if claims.NotBefore > 0 {
		now := time.Now().Unix()
		if now < claims.NotBefore {
			return nil, errors.New("token not valid yet")
		}
	}

	return &claims, nil
}

// ValidateToken parses a JWT and validates its HMAC-SHA256 signature when secret is provided.
func ValidateToken(tokenString string, secret []byte) (*Claims, error) {
	claims, err := ParseJWT(tokenString)
	if err != nil {
		return nil, err
	}

	if len(secret) > 0 {
		parts := strings.Split(tokenString, ".")
		if len(parts) != 3 {
			return nil, errors.New("invalid JWT structure")
		}

		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte(parts[0] + "." + parts[1]))
		expectedSig := mac.Sum(nil)

		actualSig, err := decodeBase64URL(parts[2])
		if err != nil {
			return nil, fmt.Errorf("failed to decode signature: %w", err)
		}

		if subtle.ConstantTimeCompare(expectedSig, actualSig) != 1 {
			return nil, errors.New("invalid token signature")
		}
	}

	return claims, nil
}

// GenerateTestToken produces a signed HS256 JWT for testing.
func GenerateTestToken(claims *Claims, secret ...[]byte) (string, error) {
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	if len(secret) == 0 || len(secret[0]) == 0 {
		header["alg"] = "none"
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	var sigB64 string
	if len(secret) > 0 && len(secret[0]) > 0 {
		mac := hmac.New(sha256.New, secret[0])
		mac.Write([]byte(headerB64 + "." + payloadB64))
		sigB64 = base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}

	return fmt.Sprintf("%s.%s.%s", headerB64, payloadB64, sigB64), nil
}

// WithActorContext populates context with the OIDC actor subject, email, and RBAC role.
func WithActorContext(ctx context.Context, claims *Claims) context.Context {
	actorID := claims.Subject
	if actorID == "" {
		actorID = claims.Email
	}
	if actorID == "" {
		actorID = "unknown"
	}

	role := claims.PrimaryRole()

	ctx = context.WithValue(ctx, ActorTypeKey, "oidc")
	ctx = context.WithValue(ctx, ActorIDKey, actorID)
	if claims.Email != "" {
		ctx = context.WithValue(ctx, ActorEmailKey, claims.Email)
	}
	if role != "" {
		ctx = context.WithValue(ctx, ActorRoleKey, string(role))
		ctx = rbac.WithRole(ctx, role)
	}

	return ctx
}

// GetActorEmail extracts the actor email from the context.
func GetActorEmail(ctx context.Context) string {
	if email, ok := ctx.Value(ActorEmailKey).(string); ok {
		return email
	}
	return ""
}

// GetActorRole extracts the actor role from the context.
func GetActorRole(ctx context.Context) rbac.Role {
	if role, ok := rbac.FromContext(ctx); ok {
		return role
	}
	if s, ok := ctx.Value(ActorRoleKey).(string); ok {
		return rbac.Role(s)
	}
	return ""
}

// OIDCValidator manages token verification and validation options.
type OIDCValidator struct {
	Secret         []byte
	ExpectedIssuer string
	ExpectedAud    string
	SkipExpiry     bool
}

// NewOIDCValidator creates an OIDCValidator instance.
func NewOIDCValidator(secret ...[]byte) *OIDCValidator {
	v := &OIDCValidator{}
	if len(secret) > 0 {
		v.Secret = secret[0]
	}
	return v
}

// Validate verifies and returns the claims of the token.
func (v *OIDCValidator) Validate(tokenString string) (*Claims, error) {
	claims, err := ValidateToken(tokenString, v.Secret)
	if err != nil {
		return nil, err
	}

	if v.ExpectedIssuer != "" && claims.Issuer != v.ExpectedIssuer {
		return nil, fmt.Errorf("unexpected issuer: got '%s', want '%s'", claims.Issuer, v.ExpectedIssuer)
	}

	return claims, nil
}

// Middleware creates an HTTP middleware that extracts and validates OIDC Bearer tokens.
func (v *OIDCValidator) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := ExtractBearerToken(r)
			if err != nil {
				http.Error(w, `{"error":{"code":"`+domain.ErrCodeUnauthorized+`","message":"`+err.Error()+`"}}`, http.StatusUnauthorized)
				return
			}

			claims, err := v.Validate(token)
			if err != nil {
				http.Error(w, `{"error":{"code":"`+domain.ErrCodeUnauthorized+`","message":"Invalid token: `+err.Error()+`"}}`, http.StatusUnauthorized)
				return
			}

			ctx := WithActorContext(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OIDCMiddleware creates an HTTP middleware for OIDC Bearer token authentication.
func OIDCMiddleware(validator ...*OIDCValidator) func(http.Handler) http.Handler {
	var v *OIDCValidator
	if len(validator) > 0 && validator[0] != nil {
		v = validator[0]
	} else {
		v = NewOIDCValidator()
	}
	return v.Middleware()
}

// CombinedAuthMiddleware supports both API key authentication and OIDC Bearer JWTs.
func CombinedAuthMiddleware(expectedAPIKey string, validator ...*OIDCValidator) func(http.Handler) http.Handler {
	var v *OIDCValidator
	if len(validator) > 0 && validator[0] != nil {
		v = validator[0]
	} else {
		v = NewOIDCValidator()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Check for X-API-Key
			key := r.Header.Get("X-API-Key")
			if key != "" && expectedAPIKey != "" && subtle.ConstantTimeCompare([]byte(key), []byte(expectedAPIKey)) == 1 {
				ctx := context.WithValue(r.Context(), ActorTypeKey, "api_key")
				ctx = context.WithValue(ctx, ActorIDKey, "admin")
				ctx = context.WithValue(ctx, ActorRoleKey, string(rbac.RoleAdmin))
				ctx = rbac.WithRole(ctx, rbac.RoleAdmin)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 2. Check Authorization header
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				bearerToken := strings.TrimPrefix(authHeader, "Bearer ")

				// Check if bearer token matches API key
				if expectedAPIKey != "" && subtle.ConstantTimeCompare([]byte(bearerToken), []byte(expectedAPIKey)) == 1 {
					ctx := context.WithValue(r.Context(), ActorTypeKey, "api_key")
					ctx = context.WithValue(ctx, ActorIDKey, "admin")
					ctx = context.WithValue(ctx, ActorRoleKey, string(rbac.RoleAdmin))
					ctx = rbac.WithRole(ctx, rbac.RoleAdmin)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				// Otherwise attempt OIDC JWT validation
				claims, err := v.Validate(bearerToken)
				if err == nil {
					ctx := WithActorContext(r.Context(), claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				http.Error(w, `{"error":{"code":"`+domain.ErrCodeUnauthorized+`","message":"Invalid token: `+err.Error()+`"}}`, http.StatusUnauthorized)
				return
			}

			http.Error(w, `{"error":{"code":"`+domain.ErrCodeUnauthorized+`","message":"Invalid or missing authentication credentials"}}`, http.StatusUnauthorized)
		})
	}
}
