package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	claimSubject           = "sub"
	claimUserID            = "user_id"
	claimTenantID          = "tenant_id"
	claimChannelIdentityID = "channel_identity_id"
	claimType              = "typ"
	claimBotID             = "bot_id"
	claimChatID            = "chat_id"
	claimRouteID           = "route_id"
	chatTokenType          = "chat_route"
	serviceTokenType       = "service"
)

// JWTMiddleware returns a JWT auth middleware configured for HS256 tokens.
func JWTMiddleware(secret string, skipper middleware.Skipper) echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		SigningKey:    []byte(secret),
		SigningMethod: "HS256",
		TokenLookup:   "header:Authorization:Bearer ,query:token",
		Skipper:       skipper,
		NewClaimsFunc: func(_ echo.Context) jwt.Claims {
			return jwt.MapClaims{}
		},
	})
}

// UserIDFromContext extracts the user id from JWT claims.
func UserIDFromContext(c echo.Context) (string, error) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok || token == nil || !token.Valid {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid token claims")
	}
	if err := requireInteractiveUserToken(claims); err != nil {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "user token required")
	}
	if userID := claimString(claims, claimUserID); userID != "" {
		return userID, nil
	}
	if userID := claimString(claims, claimSubject); userID != "" {
		return userID, nil
	}
	return "", echo.NewHTTPError(http.StatusUnauthorized, "user id missing")
}

// TenantIDFromContext extracts the tenant id from JWT claims.
func TenantIDFromContext(c echo.Context) (string, error) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok || token == nil || !token.Valid {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid token claims")
	}
	if err := requireInteractiveUserToken(claims); err != nil {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "user token required")
	}
	if tenantID := claimString(claims, claimTenantID); tenantID != "" {
		return tenantID, nil
	}
	return "", echo.NewHTTPError(http.StatusUnauthorized, "tenant id missing")
}

// TokenTypeFromContext extracts the custom token type from JWT claims.
// An empty string means the token is an unscoped user token.
func TokenTypeFromContext(c echo.Context) (string, error) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok || token == nil || !token.Valid {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "invalid token claims")
	}
	return strings.TrimSpace(claimString(claims, claimType)), nil
}

func requireInteractiveUserToken(claims jwt.MapClaims) error {
	if strings.TrimSpace(claimString(claims, claimType)) != "" {
		return errors.New("interactive user token required")
	}
	return nil
}

// GenerateToken creates a signed JWT for the user.
func GenerateToken(userID, secret string, expiresIn time.Duration) (string, time.Time, error) {
	return generateToken(userID, secret, expiresIn, "")
}

// GenerateServiceToken creates a signed JWT for internal service-triggered flows.
// Service tokens carry a non-empty type so user-only endpoints can reject them.
func GenerateServiceToken(userID, secret string, expiresIn time.Duration) (string, time.Time, error) {
	return generateToken(userID, secret, expiresIn, serviceTokenType)
}

func generateToken(userID, secret string, expiresIn time.Duration, tokenType string) (string, time.Time, error) {
	if strings.TrimSpace(userID) == "" {
		return "", time.Time{}, errors.New("user id is required")
	}
	if strings.TrimSpace(secret) == "" {
		return "", time.Time{}, errors.New("jwt secret is required")
	}
	if expiresIn <= 0 {
		return "", time.Time{}, errors.New("jwt expires in must be positive")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)
	claims := jwt.MapClaims{
		claimSubject:  userID,
		claimUserID:   userID,
		claimTenantID: userID,
		"iat":         now.Unix(),
		"exp":         expiresAt.Unix(),
	}
	if strings.TrimSpace(tokenType) != "" {
		claims[claimType] = strings.TrimSpace(tokenType)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// ChatToken holds the claims for a chat-based JWT used for route-based reply.
type ChatToken struct {
	BotID             string
	ChatID            string
	RouteID           string
	UserID            string
	ChannelIdentityID string
}

// GenerateChatToken creates a signed JWT for chat route reply.
func GenerateChatToken(info ChatToken, secret string, expiresIn time.Duration) (string, time.Time, error) {
	if strings.TrimSpace(info.BotID) == "" {
		return "", time.Time{}, errors.New("bot id is required")
	}
	if strings.TrimSpace(info.ChatID) == "" {
		return "", time.Time{}, errors.New("chat id is required")
	}
	if strings.TrimSpace(info.UserID) == "" {
		info.UserID = strings.TrimSpace(info.ChannelIdentityID)
	}
	if strings.TrimSpace(info.UserID) == "" {
		return "", time.Time{}, errors.New("user id is required")
	}
	if strings.TrimSpace(secret) == "" {
		return "", time.Time{}, errors.New("jwt secret is required")
	}
	if expiresIn <= 0 {
		return "", time.Time{}, errors.New("jwt expires in must be positive")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)
	claims := jwt.MapClaims{
		claimType:              chatTokenType,
		claimBotID:             info.BotID,
		claimChatID:            info.ChatID,
		claimRouteID:           info.RouteID,
		claimUserID:            info.UserID,
		claimChannelIdentityID: info.ChannelIdentityID,
		"iat":                  now.Unix(),
		"exp":                  expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// ChatTokenFromContext extracts the chat token claims from context.
func ChatTokenFromContext(c echo.Context) (ChatToken, error) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok || token == nil || !token.Valid {
		return ChatToken{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ChatToken{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid token claims")
	}
	if claimString(claims, claimType) != chatTokenType {
		return ChatToken{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid chat token")
	}
	info := ChatToken{
		BotID:             claimString(claims, claimBotID),
		ChatID:            claimString(claims, claimChatID),
		RouteID:           claimString(claims, claimRouteID),
		UserID:            claimString(claims, claimUserID),
		ChannelIdentityID: claimString(claims, claimChannelIdentityID),
	}
	if strings.TrimSpace(info.UserID) == "" {
		info.UserID = strings.TrimSpace(info.ChannelIdentityID)
	}
	return info, nil
}

// RefreshTokenFromContext extracts the current token from context and issues a new one
// with the same claims but a renewed expiration time.
func RefreshTokenFromContext(c echo.Context, secret string, defaultExpiresIn time.Duration) (string, time.Time, error) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok || token == nil || !token.Valid {
		return "", time.Time{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", time.Time{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid token claims")
	}
	if strings.TrimSpace(claimString(claims, claimType)) != "" {
		return "", time.Time{}, echo.NewHTTPError(http.StatusUnauthorized, "user token required")
	}

	// Calculate original duration if possible
	expiresIn := defaultExpiresIn
	if expRaw, ok := claims["exp"].(float64); ok {
		if iatRaw, ok := claims["iat"].(float64); ok {
			duration := time.Duration(expRaw-iatRaw) * time.Second
			if duration > 0 {
				expiresIn = duration
			}
		}
	}

	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)

	// Create new claims, copying over existing ones but updating time bounds
	newClaims := jwt.MapClaims{}
	for k, v := range claims {
		newClaims[k] = v
	}
	newClaims["iat"] = now.Unix()
	newClaims["exp"] = expiresAt.Unix()
	if strings.TrimSpace(claimString(newClaims, claimTenantID)) == "" {
		if tenantID := strings.TrimSpace(claimString(newClaims, claimUserID)); tenantID != "" {
			newClaims[claimTenantID] = tenantID
		} else if tenantID := strings.TrimSpace(claimString(newClaims, claimSubject)); tenantID != "" {
			newClaims[claimTenantID] = tenantID
		}
	}

	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	signed, err := newToken.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}

func claimString(claims jwt.MapClaims, key string) string {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(raw)
	}
}
